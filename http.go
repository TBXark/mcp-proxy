package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

// metricsMiddleware adds Prometheus metrics for HTTP requests
func metricsMiddleware() MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			path := r.URL.Path
			method := r.Method

			// Wrap the response writer to capture status code and response size
			rww := newResponseWriterWrapper(w)

			// Track request size
			if r.ContentLength > 0 {
				GetMetrics().requestSizeBytes.WithLabelValues(method, path).Observe(float64(r.ContentLength))
			}

			// Track in-progress requests
			GetMetrics().requestsInProgress.WithLabelValues(method, path).Inc()
			defer GetMetrics().requestsInProgress.WithLabelValues(method, path).Dec()

			// Call the next handler
			next.ServeHTTP(rww, r)

			// Record metrics after the request is complete
			duration := time.Since(start).Seconds()
			status := rww.statusCode

			GetMetrics().requestDuration.WithLabelValues(method, path).Observe(duration)
			GetMetrics().requestsTotal.WithLabelValues(method, path, fmt.Sprintf("%d", status)).Inc()
			
			if rww.bytesWritten > 0 {
				GetMetrics().responseSizeBytes.WithLabelValues(method, path).Observe(float64(rww.bytesWritten))
			}
		})
	}
}

// responseWriterWrapper wraps an http.ResponseWriter to capture metrics
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func newResponseWriterWrapper(w http.ResponseWriter) *responseWriterWrapper {
	return &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rww *responseWriterWrapper) WriteHeader(statusCode int) {
	rww.statusCode = statusCode
	rww.ResponseWriter.WriteHeader(statusCode)
}

func (rww *responseWriterWrapper) Write(p []byte) (int, error) {
	n, err := rww.ResponseWriter.Write(p)
	rww.bytesWritten += int64(n)
	return n, err
}

func startHTTPServer(config *Config) error {
	baseURL, err := url.Parse(config.McpProxy.BaseURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errorGroup errgroup.Group
	httpMux := http.NewServeMux()
	
	// Configure metrics based on configuration
	metricsEnabled := true
	metricsPath := "/metrics"
	
	if config.McpProxy.Options != nil && config.McpProxy.Options.Metrics != nil {
		metricsEnabled = config.McpProxy.Options.Metrics.Enabled && !config.McpProxy.Options.Metrics.DisableEndpoint
		if config.McpProxy.Options.Metrics.EndpointPath != "" {
			metricsPath = config.McpProxy.Options.Metrics.EndpointPath
		}
	}
	
	// Add Prometheus metrics endpoint if enabled
	if metricsEnabled {
		log.Printf("Adding metrics endpoint at %s", metricsPath)
		httpMux.Handle(metricsPath, promhttp.Handler())
	}
	
	// Add a simple health check endpoint
	httpMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	
	httpServer := &http.Server{
		Addr:    config.McpProxy.Addr,
		Handler: httpMux,
	}
	info := mcp.Implementation{
		Name:    config.McpProxy.Name,
		Version: config.McpProxy.Version,
	}

	for name, clientConfig := range config.McpServers {
		mcpClient, err := newMCPClient(name, clientConfig)
		if err != nil {
			log.Fatalf("<%s> Failed to create client: %v", name, err)
		}
		server := newMCPServer(name, config.McpProxy.Version, config.McpProxy.BaseURL, clientConfig)
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
			
			// Add metrics middleware if enabled
			metricsEnabled := true
			if config.McpProxy.Options != nil && config.McpProxy.Options.Metrics != nil {
				metricsEnabled = config.McpProxy.Options.Metrics.Enabled
			}
			if metricsEnabled {
				middlewares = append(middlewares, metricsMiddleware())
			}
			
			if clientConfig.Options.LogEnabled.OrElse(false) {
				middlewares = append(middlewares, loggerMiddleware(name))
			}
			if len(clientConfig.Options.AuthTokens) > 0 {
				middlewares = append(middlewares, newAuthMiddleware(clientConfig.Options.AuthTokens))
			}
			mcpRoute := path.Join(baseURL.Path, name)
			if !strings.HasPrefix(mcpRoute, "/") {
				mcpRoute = "/" + mcpRoute
			}
			if !strings.HasSuffix(mcpRoute, "/") {
				mcpRoute += "/"
			}
			httpMux.Handle(mcpRoute, chainMiddleware(server.sseServer, middlewares...))
			httpServer.RegisterOnShutdown(func() {
				log.Printf("<%s> Shutting down", name)
				_ = mcpClient.Close()
			})
			return nil
		})
	}

	// Add /paths endpoint to show available paths
	httpMux.HandleFunc("/paths", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		// Basic endpoints
		paths := map[string]string{
			"/health": "Health check endpoint",
			"/paths": "List of available API paths",
		}
		
		// Add metrics endpoint if enabled
		if metricsEnabled {
			paths[metricsPath] = "Prometheus metrics endpoint"
		}
		
		// Add MCP server paths with proper SSE endpoints
		for name := range config.McpServers {
			// Base route for the MCP server
			mcpRoute := path.Join(baseURL.Path, name)
			if !strings.HasPrefix(mcpRoute, "/") {
				mcpRoute = "/" + mcpRoute
			}
			if !strings.HasSuffix(mcpRoute, "/") {
				mcpRoute += "/"
			}
			
			// Add SSE endpoint (this is the main connection point)
			sseEndpoint := mcpRoute + "sse"
			paths[sseEndpoint] = fmt.Sprintf("SSE connection endpoint for %s MCP service", name)
			
			// Add Message endpoint (used internally by the SSE connection)
			messageEndpoint := mcpRoute + "message"
			paths[messageEndpoint] = fmt.Sprintf("Message endpoint for %s MCP service (used internally)", name)
		}
		
		response, _ := json.MarshalIndent(paths, "", "  ")
		w.Write(response)
	})
	
	go func() {
		err := errorGroup.Wait()
		if err != nil {
			log.Fatalf("Failed to add clients: %v", err)
		}
		log.Printf("All clients initialized")
		
		// Log available paths
		log.Println("Available API paths:")
		for name := range config.McpServers {
			mcpRoute := path.Join(baseURL.Path, name)
			if !strings.HasPrefix(mcpRoute, "/") {
				mcpRoute = "/" + mcpRoute
			}
			if !strings.HasSuffix(mcpRoute, "/") {
				mcpRoute += "/"
			}
			
			// Log the SSE endpoint (this is the main connection point clients should use)
			sseEndpoint := mcpRoute + "sse"
			log.Printf("- MCP SSE Endpoint: %s", sseEndpoint)
			
			// Log the message endpoint
			messageEndpoint := mcpRoute + "message"
			log.Printf("  └─ Message Endpoint: %s (used internally)", messageEndpoint)
		}
		log.Printf("- Health Check: /health")
		if metricsEnabled {
			log.Printf("- Metrics: %s", metricsPath)
		}
		log.Printf("- API Paths: /paths")
	}()

	go func() {
		log.Printf("Starting SSE server")
		log.Printf("SSE server listening on %s", config.McpProxy.Addr)
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

	err = httpServer.Shutdown(shutdownCtx)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}