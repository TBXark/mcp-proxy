package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Client struct {
	name            string
	needPing        bool
	needManualStart bool
	client          *client.Client
	options         *OptionsV2
}

func newMCPClient(name string, conf *MCPClientConfigV2) (*Client, error) {
	clientInfo, pErr := parseMCPClientConfigV2(conf)
	if pErr != nil {
		return nil, pErr
	}
	switch v := clientInfo.(type) {
	case *StdioMCPClientConfig:
		envs := make([]string, 0, len(v.Env))
		for kk, vv := range v.Env {
			envs = append(envs, fmt.Sprintf("%s=%s", kk, vv))
		}
		mcpClient, err := client.NewStdioMCPClient(v.Command, envs, v.Args...)
		if err != nil {
			return nil, err
		}

		return &Client{
			name:    name,
			client:  mcpClient,
			options: conf.Options,
		}, nil
	case *SSEMCPClientConfig:
		var options []transport.ClientOption
		if len(v.Headers) > 0 {
			options = append(options, client.WithHeaders(v.Headers))
		}
		mcpClient, err := client.NewSSEMCPClient(v.URL, options...)
		if err != nil {
			return nil, err
		}
		return &Client{
			name:            name,
			needPing:        true,
			needManualStart: true,
			client:          mcpClient,
			options:         conf.Options,
		}, nil
	case *StreamableMCPClientConfig:
		var options []transport.StreamableHTTPCOption
		if len(v.Headers) > 0 {
			options = append(options, transport.WithHTTPHeaders(v.Headers))
		}
		if v.Timeout > 0 {
			options = append(options, transport.WithHTTPTimeout(v.Timeout))
		}
		mcpClient, err := client.NewStreamableHttpClient(v.URL, options...)
		if err != nil {
			return nil, err
		}
		return &Client{
			name:            name,
			needPing:        true,
			needManualStart: true,
			client:          mcpClient,
			options:         conf.Options,
		}, nil
	}
	return nil, errors.New("invalid client type")
}

func (c *Client) addToMCPServer(ctx context.Context, clientInfo mcp.Implementation, mcpServer *server.MCPServer) error {
	if c.needManualStart {
		err := c.client.Start(ctx)
		if err != nil {
			return err
		}
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = clientInfo
	initRequest.Params.Capabilities = mcp.ClientCapabilities{
		Experimental: make(map[string]interface{}),
		Roots:        nil,
		Sampling:     nil,
	}
	_, err := c.client.Initialize(ctx, initRequest)
	if err != nil {
		return err
	}
	log.Printf("<%s> Successfully initialized MCP client", c.name)

	err = c.addToolsToServer(ctx, mcpServer)
	if err != nil {
		return err
	}
	_ = c.addPromptsToServer(ctx, mcpServer)
	_ = c.addResourcesToServer(ctx, mcpServer)
	_ = c.addResourceTemplatesToServer(ctx, mcpServer)

	if c.needPing {
		go c.startPingTask(ctx)
	}
	return nil
}

func (c *Client) startPingTask(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
PingLoop:
	for {
		select {
		case <-ctx.Done():
			log.Printf("<%s> Context done, stopping ping", c.name)
			break PingLoop
		case <-ticker.C:
			_ = c.client.Ping(ctx)
		}
	}
}

func (c *Client) addToolsToServer(ctx context.Context, mcpServer *server.MCPServer) error {
	toolsRequest := mcp.ListToolsRequest{}
	filterSet := make(map[string]struct{})
	var filterMode string // Store the validated filter mode
	applyFilter := false  // Whether to apply filter rules

	if c.options != nil && c.options.ToolFilter != nil && len(c.options.ToolFilter.List) > 0 {
		// List is provided, now validate the mode explicitly
		mode := strings.ToLower(c.options.ToolFilter.Mode)
		if mode == "allow" || mode == "block" {
			// Mode is valid, prepare to apply filter
			applyFilter = true
			filterMode = mode // Store the valid mode
			for _, toolName := range c.options.ToolFilter.List {
				filterSet[toolName] = struct{}{}
			}
		} else {
			// Mode is missing or invalid, ignore the filter configuration
			if c.options.ToolFilter.Mode == "" {
				log.Printf("<%s> WARNING: toolFilter list provided but mode is missing. Ignoring toolFilter configuration for this server. Mode must be 'allow' or 'block'.", c.name)
			} else {
				log.Printf("<%s> WARNING: Invalid toolFilter mode '%s' provided. Ignoring toolFilter configuration for this server. Mode must be 'allow' or 'block'.", c.name, c.options.ToolFilter.Mode)
			}
			// applyFilter remains false
		}
	}

	for {
		tools, err := c.client.ListTools(ctx, toolsRequest)
		if err != nil {
			return err
		}
		if len(tools.Tools) == 0 {
			break
		}
		log.Printf("<%s> Successfully listed %d tools", c.name, len(tools.Tools))
		for _, tool := range tools.Tools {
			shouldAdd := true // Default to adding the tool

			if applyFilter { // Only apply filter rules if applyFilter is true
				_, inList := filterSet[tool.Name]
				if filterMode == "allow" {
					if !inList {
						shouldAdd = false
					}
				} else { // filterMode == "block" (already validated)
					if inList {
						shouldAdd = false
					}
				}
			}

			if shouldAdd {
				log.Printf("<%s> Adding tool %s", c.name, tool.Name)
				mcpServer.AddTool(tool, c.client.CallTool)
			} else if applyFilter { // Log skip reason only if filtering was applied and caused the skip
				if filterMode == "allow" {
					log.Printf("<%s> Skipping tool %s (not in allowlist)", c.name, tool.Name)
				} else { // filterMode == "block"
					log.Printf("<%s> Skipping blocked tool %s", c.name, tool.Name)
				}
			}
		}
		if tools.NextCursor == "" {
			break
		}
		toolsRequest.Params.Cursor = tools.NextCursor
	}
	return nil
}

func (c *Client) addPromptsToServer(ctx context.Context, mcpServer *server.MCPServer) error {
	promptsRequest := mcp.ListPromptsRequest{}
	for {
		prompts, err := c.client.ListPrompts(ctx, promptsRequest)
		if err != nil {
			return err
		}
		if len(prompts.Prompts) == 0 {
			break
		}
		log.Printf("<%s> Successfully listed %d prompts", c.name, len(prompts.Prompts))
		for _, prompt := range prompts.Prompts {
			log.Printf("<%s> Adding prompt %s", c.name, prompt.Name)
			mcpServer.AddPrompt(prompt, c.client.GetPrompt)
		}
		if prompts.NextCursor == "" {
			break
		}
		promptsRequest.Params.Cursor = prompts.NextCursor
	}
	return nil
}

func (c *Client) addResourcesToServer(ctx context.Context, mcpServer *server.MCPServer) error {
	resourcesRequest := mcp.ListResourcesRequest{}
	for {
		resources, err := c.client.ListResources(ctx, resourcesRequest)
		if err != nil {
			return err
		}
		if len(resources.Resources) == 0 {
			break
		}
		log.Printf("<%s> Successfully listed %d resources", c.name, len(resources.Resources))
		for _, resource := range resources.Resources {
			log.Printf("<%s> Adding resource %s", c.name, resource.Name)
			mcpServer.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				readResource, e := c.client.ReadResource(ctx, request)
				if e != nil {
					return nil, e
				}
				return readResource.Contents, nil
			})
		}
		if resources.NextCursor == "" {
			break
		}
		resourcesRequest.Params.Cursor = resources.NextCursor

	}
	return nil
}

func (c *Client) addResourceTemplatesToServer(ctx context.Context, mcpServer *server.MCPServer) error {
	resourceTemplatesRequest := mcp.ListResourceTemplatesRequest{}
	for {
		resourceTemplates, err := c.client.ListResourceTemplates(ctx, resourceTemplatesRequest)
		if err != nil {
			return err
		}
		if len(resourceTemplates.ResourceTemplates) == 0 {
			break
		}
		log.Printf("<%s> Successfully listed %d resource templates", c.name, len(resourceTemplates.ResourceTemplates))
		for _, resourceTemplate := range resourceTemplates.ResourceTemplates {
			log.Printf("<%s> Adding resource template %s", c.name, resourceTemplate.Name)
			mcpServer.AddResourceTemplate(resourceTemplate, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				readResource, e := c.client.ReadResource(ctx, request)
				if e != nil {
					return nil, e
				}
				return readResource.Contents, nil
			})
		}
		if resourceTemplates.NextCursor == "" {
			break
		}
		resourceTemplatesRequest.Params.Cursor = resourceTemplates.NextCursor
	}
	return nil
}

func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

type Server struct {
	tokens    []string
	mcpServer *server.MCPServer
	sseServer *server.SSEServer
}

func newMCPServer(name, version, baseURL string, clientConfig *MCPClientConfigV2) *Server {
	serverOpts := []server.ServerOption{
		server.WithResourceCapabilities(true, true),
		server.WithRecovery(),
	}

	if clientConfig.Options.LogEnabled.OrElse(false) {
		serverOpts = append(serverOpts, server.WithLogging())
	}
	mcpServer := server.NewMCPServer(
		name,
		version,
		serverOpts...,
	)
	sseServer := server.NewSSEServer(mcpServer,
		server.WithBasePath(name),
		server.WithBaseURL(baseURL),
	)

	srv := &Server{
		mcpServer: mcpServer,
		sseServer: sseServer,
	}

	if clientConfig.Options != nil && len(clientConfig.Options.AuthTokens) > 0 {
		srv.tokens = clientConfig.Options.AuthTokens
	}

	return srv
}
