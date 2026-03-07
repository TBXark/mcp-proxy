package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"
)

// serverRegistry holds connected MCP servers (static + dynamic) for routing.
type serverRegistry struct {
	mu           sync.RWMutex
	entries      map[string]*registryEntry
	dynamicNames map[string]struct{} // names registered via API
}

type registryEntry struct {
	handler      http.Handler
	client       *Client
	shutdownFunc func()
}

func newServerRegistry() *serverRegistry {
	return &serverRegistry{
		entries:      make(map[string]*registryEntry),
		dynamicNames: make(map[string]struct{}),
	}
}

func (r *serverRegistry) add(name string, e *registryEntry, dynamic bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[name] = e
	if dynamic {
		r.dynamicNames[name] = struct{}{}
	}
}

func (r *serverRegistry) get(name string) (*registryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return e, ok
}

func (r *serverRegistry) listDynamic() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.dynamicNames))
	for n := range r.dynamicNames {
		names = append(names, n)
	}
	return names
}

// applyOptionDefaults applies proxy-level option defaults to a client config (mirrors load()).
func applyOptionDefaults(proxyOpts *OptionsV2, clientConfig *MCPClientConfigV2) {
	if proxyOpts == nil || clientConfig == nil {
		return
	}
	if clientConfig.Options == nil {
		clientConfig.Options = &OptionsV2{}
	}
	if clientConfig.Options.AuthTokens == nil {
		clientConfig.Options.AuthTokens = proxyOpts.AuthTokens
	}
	if !clientConfig.Options.PanicIfInvalid.Present() {
		clientConfig.Options.PanicIfInvalid = proxyOpts.PanicIfInvalid
	}
	if !clientConfig.Options.LogEnabled.Present() {
		clientConfig.Options.LogEnabled = proxyOpts.LogEnabled
	}
}

type MiddlewareFunc func(http.Handler) http.Handler

func chainMiddleware(h http.Handler, middlewares ...MiddlewareFunc) http.Handler {
	for _, mw := range middlewares {
		h = mw(h)
	}
	return h
}

func newAuthMiddleware(tokens []string) MiddlewareFunc {
	tokenSet := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		tokenSet[token] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(tokens) != 0 {
				token := r.Header.Get("Authorization")
				token = strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))
				if token == "" {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				if _, ok := tokenSet[token]; !ok {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func loggerMiddleware(prefix string) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("<%s> Request [%s] %s", prefix, r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}

func recoverMiddleware(prefix string) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("<%s> Recovered from panic: %v", prefix, err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// registerServerRequest is the JSON body for POST /servers/register.
type registerServerRequest struct {
	Name   string             `json:"name"`
	Config *MCPClientConfigV2 `json:"config"`
}

func serversListHandler(config *Config, registry *serverRegistry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		serverSet := make(map[string]struct{})
		for name, clientConfig := range config.McpServers {
			if clientConfig.Options != nil && clientConfig.Options.Disabled {
				continue
			}
			serverSet[name] = struct{}{}
		}
		for _, name := range registry.listDynamic() {
			serverSet[name] = struct{}{}
		}
		servers := make([]string, 0, len(serverSet))
		for name := range serverSet {
			servers = append(servers, name)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string][]string{"servers": servers})
	})
}

// registrationTimeout is how long we wait for the MCP handshake when registering a server.
const registrationTimeout = 90 * time.Second

func serversRegisterHandler(config *Config, registry *serverRegistry, httpServer *http.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		var req registerServerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if req.Config == nil {
			http.Error(w, "config is required", http.StatusBadRequest)
			return
		}
		// Prevent overwriting a server that already exists in config or was already registered
		if _, inConfig := config.McpServers[req.Name]; inConfig {
			http.Error(w, "server name already exists in config", http.StatusConflict)
			return
		}
		if _, ok := registry.get(req.Name); ok {
			http.Error(w, "server already registered", http.StatusConflict)
			return
		}

		clientConfig := req.Config
		applyOptionDefaults(config.McpProxy.Options, clientConfig)
		if clientConfig.Options == nil {
			clientConfig.Options = &OptionsV2{}
		}

		mcpClient, err := newMCPClient(req.Name, clientConfig)
		if err != nil {
			log.Printf("<%s> Register: failed to create client: %v", req.Name, err)
			http.Error(w, "Failed to create client: "+err.Error(), http.StatusBadRequest)
			return
		}
		server, err := newMCPServer(req.Name, config.McpProxy, clientConfig)
		if err != nil {
			_ = mcpClient.Close()
			log.Printf("<%s> Register: failed to create server: %v", req.Name, err)
			http.Error(w, "Failed to create server: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Use a dedicated timeout so the handshake is not tied to the HTTP client's timeout.
		opCtx, opCancel := context.WithTimeout(context.Background(), registrationTimeout)
		defer opCancel()

		info := mcp.Implementation{Name: config.McpProxy.Name}
		if err := mcpClient.addToMCPServer(opCtx, info, server.mcpServer); err != nil {
			_ = mcpClient.Close()
			log.Printf("<%s> Register: failed to add client to server: %v", req.Name, err)
			errMsg := err.Error()
			if strings.Contains(errMsg, "transport closed") {
				errMsg = "MCP transport closed during handshake. For stdio servers, ensure the command runs (e.g. run it in a terminal to see errors). For URL-based servers, check the endpoint and network."
			}
			http.Error(w, "Failed to connect: "+errMsg, http.StatusInternalServerError)
			return
		}

		middlewares := make([]MiddlewareFunc, 0)
		middlewares = append(middlewares, recoverMiddleware(req.Name))
		if clientConfig.Options.LogEnabled.OrElse(false) {
			middlewares = append(middlewares, loggerMiddleware(req.Name))
		}
		if len(clientConfig.Options.AuthTokens) > 0 {
			middlewares = append(middlewares, newAuthMiddleware(clientConfig.Options.AuthTokens))
		}
		handler := chainMiddleware(server.handler, middlewares...)

		entry := &registryEntry{
			handler: handler,
			client:  mcpClient,
			shutdownFunc: func() {
				log.Printf("<%s> Shutting down (dynamic)", req.Name)
				_ = mcpClient.Close()
			},
		}
		registry.add(req.Name, entry, true)
		httpServer.RegisterOnShutdown(entry.shutdownFunc)

		log.Printf("<%s> Registered dynamically", req.Name)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"name": req.Name, "status": "registered"})
	})
}

// basePath returns the proxy base path (always starts with /).
func basePath(baseURL *url.URL) string {
	p := strings.TrimSuffix(baseURL.Path, "/")
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// relPath returns the path relative to base (no leading slash).
func relPath(fullPath, base string) string {
	s := strings.TrimPrefix(fullPath, base)
	return strings.TrimPrefix(s, "/")
}

// mainRouter handles all requests under the proxy base path: list, register, and MCP server routes.
func mainRouter(basePathStr string, config *Config, registry *serverRegistry, httpServer *http.Server) http.Handler {
	listH := serversListHandler(config, registry)
	registerH := serversRegisterHandler(config, registry, httpServer)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := relPath(r.URL.Path, basePathStr)
		segments := strings.SplitN(rel, "/", 3)
		first := ""
		if len(segments) > 0 {
			first = segments[0]
		}
		switch first {
		case "servers":
			if len(segments) >= 2 && segments[1] == "register" {
				registerH.ServeHTTP(w, r)
				return
			}
			listH.ServeHTTP(w, r)
			return
		case "":
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		default:
			// first segment is server name
			entry, ok := registry.get(first)
			if !ok {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			entry.handler.ServeHTTP(w, r)
		}
	})
}

func startHTTPServer(config *Config) error {
	baseURL, uErr := url.Parse(config.McpProxy.BaseURL)
	if uErr != nil {
		return uErr
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registry := newServerRegistry()
	basePathStr := basePath(baseURL)

	httpMux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    config.McpProxy.Addr,
		Handler: httpMux,
	}
	info := mcp.Implementation{
		Name: config.McpProxy.Name,
	}

	// Mount the main router at the proxy base path so it handles /servers, /servers/register, and /<name>/...
	mountPath := basePathStr
	router := mainRouter(mountPath, config, registry, httpServer)
	if mountPath == "/" {
		httpMux.Handle("/", router)
	} else {
		httpMux.Handle(mountPath, router)
		httpMux.Handle(mountPath+"/", router)
	}
	log.Printf("List API at %s/servers", mountPath)
	log.Printf("Register API at %s/servers/register (POST)", mountPath)

	var errorGroup errgroup.Group
	for name, clientConfig := range config.McpServers {
		if clientConfig.Options != nil && clientConfig.Options.Disabled {
			log.Printf("<%s> Disabled", name)
			continue
		}
		mcpClient, err := newMCPClient(name, clientConfig)
		if err != nil {
			return err
		}
		server, err := newMCPServer(name, config.McpProxy, clientConfig)
		if err != nil {
			return err
		}
		name := name
		clientConfig := clientConfig
		errorGroup.Go(func() error {
			log.Printf("<%s> Connecting", name)
			addErr := mcpClient.addToMCPServer(ctx, info, server.mcpServer)
			if addErr != nil {
				log.Printf("<%s> Failed to add client to server: %v", name, addErr)
				if clientConfig.Options != nil && clientConfig.Options.PanicIfInvalid.OrElse(false) {
					return addErr
				}
				return nil
			}
			log.Printf("<%s> Connected", name)

			middlewares := make([]MiddlewareFunc, 0)
			middlewares = append(middlewares, recoverMiddleware(name))
			if clientConfig.Options != nil && clientConfig.Options.LogEnabled.OrElse(false) {
				middlewares = append(middlewares, loggerMiddleware(name))
			}
			if clientConfig.Options != nil && len(clientConfig.Options.AuthTokens) > 0 {
				middlewares = append(middlewares, newAuthMiddleware(clientConfig.Options.AuthTokens))
			}
			handler := chainMiddleware(server.handler, middlewares...)
			entry := &registryEntry{
				handler: handler,
				client:  mcpClient,
				shutdownFunc: func() {
					log.Printf("<%s> Shutting down", name)
					_ = mcpClient.Close()
				},
			}
			registry.add(name, entry, false)
			httpServer.RegisterOnShutdown(entry.shutdownFunc)
			log.Printf("<%s> Handling requests at %s/%s/", name, mountPath, name)
			return nil
		})
	}

	go func() {
		err := errorGroup.Wait()
		if err != nil {
			log.Fatalf("Failed to add clients: %v", err)
		}
		log.Printf("All clients initialized")
	}()

	go func() {
		log.Printf("Starting %s server", config.McpProxy.Type)
		log.Printf("%s server listening on %s", config.McpProxy.Type, config.McpProxy.Addr)
		hErr := httpServer.ListenAndServe()
		if hErr != nil && !errors.Is(hErr, http.ErrServerClosed) {
			log.Fatalf("Failed to start server: %v", hErr)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 5*time.Second)
	defer shutdownCancel()

	err := httpServer.Shutdown(shutdownCtx)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
