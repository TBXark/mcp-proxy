package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"
)

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

func newCombinedAuthMiddleware(authTokens []string, oauth2Config *OAuth2Config, oauthServer *OAuthServer) MiddlewareFunc {
	// Create token set for fast lookup
	tokenSet := make(map[string]struct{}, len(authTokens))
	for _, token := range authTokens {
		tokenSet[token] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
				return
			}

			// Check for Bearer token
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
				if token == "" {
					http.Error(w, "Missing access token", http.StatusUnauthorized)
					return
				}

				// First, try predefined auth tokens (fastest check)
				if len(authTokens) > 0 {
					if _, ok := tokenSet[token]; ok {
						log.Printf("Request authenticated with predefined bearer token")
						next.ServeHTTP(w, r)
						return
					}
				}

				// Second, try OAuth validation if enabled
				if oauth2Config != nil && oauth2Config.Enabled && oauthServer != nil {
					accessToken, valid := oauthServer.ValidateToken(token)
					if valid {
						log.Printf("Request authenticated with OAuth token for client: %s, username: %s", accessToken.ClientID, accessToken.Username)
						// Add token info to request context for potential use
						ctx := context.WithValue(r.Context(), "X-OAuth-Client-ID", accessToken.ClientID)
						ctx = context.WithValue(ctx, "X-OAuth-Scope", accessToken.Scope)
						ctx = context.WithValue(ctx, "X-OAuth-Resource", accessToken.Resource)
						ctx = context.WithValue(ctx, "X-OAuth-Username", accessToken.Username)

						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}

				// If neither auth method worked
				http.Error(w, "Invalid or expired access token", http.StatusUnauthorized)
				return
			}

			http.Error(w, "Unsupported authorization method. Use Bearer token", http.StatusUnauthorized)
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

func newServerAccessMiddleware(serverName string, userFilter *UserFilterConfig) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip access control if no user filter configured
			if userFilter == nil {
				next.ServeHTTP(w, r)
				return
			}

			username, ok := r.Context().Value("X-OAuth-Username").(string)
			if !ok || username == "" {
				// No OAuth username in context, proceed normally (this might be token-based auth)
				next.ServeHTTP(w, r)
				return
			}

			// Check if user has access to this server based on user filter
			if !userFilter.IsUserAllowed(username) {
				log.Printf("User %s denied access to server %s (mode: %s, list: %v)",
					username, serverName, userFilter.Mode, userFilter.List)
				http.Error(w, "Access denied: You don't have permission to access this server", http.StatusForbidden)
				return
			}

			log.Printf("User %s granted access to server %s", username, serverName)
			next.ServeHTTP(w, r)
		})
	}
}

func startHTTPServer(config *Config) error {
	baseURL, uErr := url.Parse(config.McpProxy.BaseURL)
	if uErr != nil {
		return uErr
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errorGroup errgroup.Group
	httpMux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    config.McpProxy.Addr,
		Handler: httpMux,
	}
	info := mcp.Implementation{
		Name: config.McpProxy.Name,
	}

	// Create OAuth 2.1 server with access control config
	var oauthAccessConfig *OAuth2Config
	if config.McpProxy.Options != nil && config.McpProxy.Options.OAuth2 != nil {
		oauthAccessConfig = config.McpProxy.Options.OAuth2
	}
	oauthServer := NewOAuthServer(config.McpProxy.BaseURL, oauthAccessConfig)
	
	// Register OAuth routes
	oauthServer.RegisterRoutes(httpMux)

	for name, clientConfig := range config.McpServers {
		mcpClient, err := newMCPClient(name, clientConfig)
		if err != nil {
			return err
		}
		server, err := newMCPServer(name, config.McpProxy, clientConfig)
		if err != nil {
			return err
		}
		errorGroup.Go(func() error {
			log.Printf("<%s> Connecting", name)
			addErr := mcpClient.addToMCPServer(ctx, info, server.mcpServer)
			if addErr != nil {
				log.Printf("<%s> Failed to add client to server: %v", name, addErr)
				if clientConfig.Options.PanicIfInvalid.OrElse(false) {
					return addErr
				}
				return nil
			}
			log.Printf("<%s> Connected", name)

			middlewares := make([]MiddlewareFunc, 0)
			middlewares = append(middlewares, recoverMiddleware(name))
			if clientConfig.Options.LogEnabled.OrElse(false) {
				middlewares = append(middlewares, loggerMiddleware(name))
			}

			// Apply combined authentication middleware (supports both predefined tokens and OAuth)
			if config.McpProxy.Type == MCPServerTypeStreamable &&
			   ((config.McpProxy.Options.OAuth2 != nil && config.McpProxy.Options.OAuth2.Enabled) || len(clientConfig.Options.AuthTokens) > 0) {
				middlewares = append(middlewares, newCombinedAuthMiddleware(
					clientConfig.Options.AuthTokens,
					config.McpProxy.Options.OAuth2,
					oauthServer,
				))
				// Add server access middleware for OAuth-enabled servers with user filters
				if config.McpProxy.Options.OAuth2 != nil && config.McpProxy.Options.OAuth2.Enabled && clientConfig.Options.UserFilter != nil {
					middlewares = append(middlewares, newServerAccessMiddleware(name, clientConfig.Options.UserFilter))
				}
			} else if len(clientConfig.Options.AuthTokens) > 0 {
				// For non-streamable transports, use simple auth middleware
				middlewares = append(middlewares, newAuthMiddleware(clientConfig.Options.AuthTokens))
			}
			mcpRoute := path.Join(baseURL.Path, name)
			log.Printf("<%s> baseURL.Path='%s', name='%s', initial mcpRoute='%s'", name, baseURL.Path, name, mcpRoute)
			
			if !strings.HasPrefix(mcpRoute, "/") {
				mcpRoute = "/" + mcpRoute
			}
			
			baseHandler := chainMiddleware(server.handler, middlewares...)
			
			// Register exact paths to avoid Go's automatic redirect behavior
			mcpRouteWithoutSlash := strings.TrimSuffix(mcpRoute, "/")
			mcpRouteWithSlash := mcpRouteWithoutSlash + "/"
			
			log.Printf("<%s> Registering exact routes: '%s' and '%s'", name, mcpRouteWithoutSlash, mcpRouteWithSlash)
			
			// Register both exact patterns
			httpMux.HandleFunc(mcpRouteWithoutSlash, func(w http.ResponseWriter, r *http.Request) {
				// Only handle exact matches to prevent Go's redirect behavior
				if r.URL.Path == mcpRouteWithoutSlash {
					baseHandler.ServeHTTP(w, r)
				} else {
					http.NotFound(w, r)
				}
			})
			
			httpMux.HandleFunc(mcpRouteWithSlash, func(w http.ResponseWriter, r *http.Request) {
				baseHandler.ServeHTTP(w, r)
			})
			
			log.Printf("<%s> Routes registered successfully", name)
			httpServer.RegisterOnShutdown(func() {
				log.Printf("<%s> Shutting down", name)
				_ = mcpClient.Close()
			})
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
