package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/TBXark/confstore"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/sync/errgroup"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var BuildVersion = "dev"

type StdioMCPClientConfig struct {
	Command string            `json:"command"`
	Env     map[string]string `json:"env"`
	Args    []string          `json:"args"`
}

type SSEMCPClientConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Timeout int64             `json:"timeout"`
}

type MCPClientType string

const (
	MCPClientTypeStdio MCPClientType = "stdio"
	MCPClientTypeSSE   MCPClientType = "sse"
)

type MCPClientConfig struct {
	Type           MCPClientType   `json:"type"`
	Config         json.RawMessage `json:"config"`
	PanicIfInvalid bool            `json:"panicIfInvalid"`
}
type SSEServerConfig struct {
	BaseURL string `json:"baseURL"`
	Addr    string `json:"addr"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Config struct {
	Server  SSEServerConfig            `json:"server"`
	Clients map[string]MCPClientConfig `json:"clients"`
}

func main() {
	conf := flag.String("config", "config.json", "path to config file or a http(s) url")
	version := flag.Bool("version", false, "print version and exit")
	help := flag.Bool("help", false, "print help and exit")
	flag.Parse()
	if *help {
		flag.Usage()
		return
	}
	if *version {
		fmt.Println(BuildVersion)
		return
	}
	config, err := confstore.Load[Config](*conf)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	start(config)
}

func start(config *Config) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errorGroup errgroup.Group
	httpMux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    config.Server.Addr,
		Handler: httpMux,
	}
	info := mcp.Implementation{
		Name:    config.Server.Name,
		Version: config.Server.Version,
	}
	for name, clientConfig := range config.Clients {
		log.Printf("Connecting to %s", name)
		mcpClient, err := newMCPClient(clientConfig)
		if err != nil {
			log.Fatalf("Failed to create MCP client: %v", err)
		}
		mcpServer := server.NewMCPServer(
			config.Server.Name,
			config.Server.Version,
			server.WithResourceCapabilities(true, true),
		)
		sseServer := server.NewSSEServer(mcpServer,
			server.WithBaseURL(config.Server.BaseURL),
			server.WithBasePath(name),
		)
		errorGroup.Go(func() error {
			addErr := addClient(ctx, info, mcpClient, mcpServer)
			if addErr != nil && clientConfig.PanicIfInvalid {
				return addErr
			}
			return nil
		})
		httpMux.Handle(fmt.Sprintf("/%s/", name), sseServer)
		httpServer.RegisterOnShutdown(func() {
			log.Printf("Closing client %s", name)
			_ = mcpClient.Close()
		})
	}
	err := errorGroup.Wait()
	if err != nil {
		log.Fatalf("Failed to add clients: %v", err)
	}

	go func() {
		log.Printf("Starting SSE server")
		log.Printf("SSE server listening on %s", config.Server.Addr)
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
	if err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}

func parseMCPClientConfig(conf MCPClientConfig) (any, error) {
	switch conf.Type {
	case MCPClientTypeStdio:
		var config StdioMCPClientConfig
		err := json.Unmarshal(conf.Config, &config)
		if err != nil {
			return nil, err
		}
		return config, nil
	case MCPClientTypeSSE:
		var config SSEMCPClientConfig
		err := json.Unmarshal(conf.Config, &config)
		if err != nil {
			return nil, err
		}
		return config, nil
	default:
		return nil, errors.New("invalid client type")
	}
}

func newMCPClient(conf MCPClientConfig) (client.MCPClient, error) {
	clientInfo, pErr := parseMCPClientConfig(conf)
	if pErr != nil {
		return nil, pErr
	}
	switch v := clientInfo.(type) {
	case StdioMCPClientConfig:
		envs := make([]string, 0, len(v.Env))
		for kk, vv := range v.Env {
			envs = append(envs, fmt.Sprintf("%s=%s", kk, vv))
		}
		return client.NewStdioMCPClient(v.Command, envs, v.Args...)
	case SSEMCPClientConfig:
		var options []client.ClientOption
		if v.Timeout > 0 {
			options = append(options, client.WithSSEReadTimeout(time.Duration(v.Timeout)*time.Second))
		}
		if len(v.Headers) > 0 {
			options = append(options, client.WithHeaders(v.Headers))
		}
		return client.NewSSEMCPClient(v.URL, options...)
	}
	return nil, errors.New("invalid client type")
}

func addClient(ctx context.Context, clientInfo mcp.Implementation, mcpClient client.MCPClient, mcpServer *server.MCPServer) error {
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = clientInfo
	_, err := mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		return err
	}
	log.Printf("Successfully initialized MCP client")

	err = addClientToolsToServer(ctx, mcpClient, mcpServer)
	if err != nil {
		return err
	}
	_ = addClientPromptsToServer(ctx, mcpClient, mcpServer)
	_ = addClientResourcesToServer(ctx, mcpClient, mcpServer)
	_ = addClientResourceTemplatesToServer(ctx, mcpClient, mcpServer)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("Context done, stopping ping")
				break
			case <-ticker.C:
				_ = mcpClient.Ping(ctx)
			}
		}
	}()
	return nil
}

func addClientToolsToServer(ctx context.Context, mcpClient client.MCPClient, mcpServer *server.MCPServer) error {
	toolsRequest := mcp.ListToolsRequest{}
	for {
		tools, err := mcpClient.ListTools(ctx, toolsRequest)
		if err != nil {
			return err
		}
		log.Printf("Successfully listed %d tools", len(tools.Tools))
		for _, tool := range tools.Tools {
			log.Printf("Adding tool %s", tool.Name)
			mcpServer.AddTool(tool, mcpClient.CallTool)
		}
		if tools.NextCursor == "" {
			break
		}
		toolsRequest.PaginatedRequest.Params.Cursor = tools.NextCursor
	}
	return nil
}

func addClientPromptsToServer(ctx context.Context, mcpClient client.MCPClient, mcpServer *server.MCPServer) error {
	promptsRequest := mcp.ListPromptsRequest{}
	for {
		prompts, err := mcpClient.ListPrompts(ctx, promptsRequest)
		if err != nil {
			return err
		}
		log.Printf("Successfully listed %d prompts", len(prompts.Prompts))
		for _, prompt := range prompts.Prompts {
			log.Printf("Adding prompt %s", prompt.Name)
			mcpServer.AddPrompt(prompt, mcpClient.GetPrompt)
		}
		if prompts.NextCursor == "" {
			break
		}
		promptsRequest.PaginatedRequest.Params.Cursor = prompts.NextCursor
	}
	return nil
}

func addClientResourcesToServer(ctx context.Context, mcpClient client.MCPClient, mcpServer *server.MCPServer) error {
	resourcesRequest := mcp.ListResourcesRequest{}
	for {
		resources, err := mcpClient.ListResources(ctx, resourcesRequest)
		if err != nil {
			return err
		}
		log.Printf("Successfully listed %d resources", len(resources.Resources))
		for _, resource := range resources.Resources {
			log.Printf("Adding resource %s", resource.Name)
			mcpServer.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				readResource, e := mcpClient.ReadResource(ctx, request)
				if e != nil {
					return nil, e
				}
				return readResource.Contents, nil
			})
		}
		if resources.NextCursor == "" {
			break
		}
		resourcesRequest.PaginatedRequest.Params.Cursor = resources.NextCursor

	}
	return nil
}

func addClientResourceTemplatesToServer(ctx context.Context, mcpClient client.MCPClient, mcpServer *server.MCPServer) error {
	resourceTemplatesRequest := mcp.ListResourceTemplatesRequest{}
	for {
		resourceTemplates, err := mcpClient.ListResourceTemplates(ctx, resourceTemplatesRequest)
		if err != nil {
			return err
		}
		log.Printf("Successfully listed %d resource templates", len(resourceTemplates.ResourceTemplates))
		for _, resourceTemplate := range resourceTemplates.ResourceTemplates {
			log.Printf("Adding resource template %s", resourceTemplate.Name)
			mcpServer.AddResourceTemplate(resourceTemplate, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				readResource, e := mcpClient.ReadResource(ctx, request)
				if e != nil {
					return nil, e
				}
				return readResource.Contents, nil
			})
		}
		if resourceTemplates.NextCursor == "" {
			break
		}
		resourceTemplatesRequest.PaginatedRequest.Params.Cursor = resourceTemplates.NextCursor
	}
	return nil
}
