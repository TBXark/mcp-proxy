package main

import (
	"context"
	
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolCallRequest is a wrapper type to allow backward compatibility 
// with the previous version of client.go
type ToolCallRequest mcp.CallToolRequest

// ToolCallResponse is a wrapper type to allow backward compatibility
// with the previous version of client.go
type ToolCallResponse struct {
	*mcp.CallToolResult
}

// CallTool adapts the client's CallTool method to work with our wrapper types
func (c *Client) CallTool(ctx context.Context, request ToolCallRequest) (ToolCallResponse, error) {
	// Convert our request to the library's request type
	mcpRequest := mcp.CallToolRequest(request)
	
	// Call the actual method
	resp, err := c.client.CallTool(ctx, mcpRequest)
	if err != nil {
		return ToolCallResponse{}, err
	}
	
	// Return the wrapped response
	return ToolCallResponse{resp}, nil
}