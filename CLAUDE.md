# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MCP Proxy is a Go-based proxy server that aggregates multiple Model Context Protocol (MCP) resource servers through a single HTTP interface. It acts as a unified gateway, collecting tools, prompts, and resources from various MCP clients and exposing them via HTTP endpoints with SSE or streamable-HTTP transport.

## Development Commands

### Build and Run
```bash
# Build the project
make build

# Run the built binary
./build/mcp-proxy --config config.json

# Build for Linux x86_64
make buildLinuxX86

# Install via Go
go install github.com/TBXark/mcp-proxy@latest
```

### Code Quality
```bash
# Run linter
make lint

# Format code and fix issues
make format
```

### Docker
```bash
# Build multi-arch Docker image
make buildImage

# Run with Docker
docker run -d -p 9090:9090 -v /path/to/config.json:/config/config.json ghcr.io/tbxark/mcp-proxy:latest
```

## Architecture Overview

### Single Package Design
All code is in the root package with clear separation of concerns across files:
- `main.go`: CLI entry point and argument parsing
- `config.go`: Configuration structures and V2 format with migration from V1
- `client.go`: MCP client management and server integration logic
- `http.go`: HTTP server with middleware chain (auth, logging, recovery)
- `oauth.go`: OAuth 2.1 authorization server implementation

### Key Patterns
- **Proxy Aggregation**: Collects capabilities from multiple upstream MCP servers
- **Transport Abstraction**: Supports three transport types seamlessly
- **Middleware Chain**: Modular HTTP middleware for cross-cutting concerns
- **Configuration Migration**: Automatic V1 to V2 config format upgrade
- **OAuth 2.1 Server**: Complete authorization server with user access control

### Transport Types
1. **stdio**: Command-line subprocess communication (e.g., npx, uvx commands)
2. **sse**: Server-Sent Events for real-time updates  
3. **streamable-http**: HTTP streaming transport

Each transport type is automatically detected based on configuration fields present.

## Configuration System

Uses V2 configuration format with backward compatibility:
- **mcpProxy**: Server settings (baseURL, addr, type, auth tokens, OAuth 2 config)
- **mcpServers**: Individual MCP client configurations
- **Tool Filtering**: Allow/block specific tools per server with `toolFilter.mode` and `toolFilter.list`
- **Per-client Auth**: Individual auth tokens override global defaults
- **OAuth 2 Client Credentials**: Supports OAuth 2 authentication for streamable HTTP transport
- **Access Control**: IP allowlist/blocklist, client approval workflows, user restrictions

Configuration can be loaded from local files or HTTP URLs. The system automatically migrates V1 configs to V2 format.

## Authentication

### OAuth 2 Client Credentials Flow
For `streamable-http` transport only:
- Enable via `options.oauth2.enabled: true` in configuration
- Full OAuth 2.1 authorization server with Dynamic Client Registration (RFC 7591)
- PKCE support for enhanced security
- Per-server OAuth discovery endpoints
- Client persistence across server restarts
- Comprehensive access control system

### Bearer Token Authentication
For all transport types:
- Configure via `options.authTokens` array in configuration
- Uses `Authorization: Bearer <token>` header format
- Falls back to this method when OAuth 2 is not configured

### Access Control Features
- **IP Restrictions**: `allowedIPs` and `blockedIPs` arrays
- **Client Management**: `allowedClients` and `blockedClients` arrays  
- **Approval Workflow**: `requireApproval` flag for manual client approval
- **Client Tracking**: Automatic logging of client IP addresses and metadata

## Important Notes

- **No Tests**: This codebase currently lacks automated tests - consider this when making changes
- **Error Handling**: Uses panic recovery middleware and optional `panicIfInvalid` per client
- **Logging**: Configurable per-client logging with request tracing
- **Health Monitoring**: Automatic ping/health checking for SSE and HTTP transport clients
- **Graceful Shutdown**: Proper signal handling for clean resource cleanup
- **Client Persistence**: OAuth clients saved to `oauth_clients.json` for persistence

## Dependencies

- `github.com/mark3labs/mcp-go`: Core MCP protocol implementation
- `github.com/TBXark/confstore`: Configuration management with HTTP loading
- `github.com/TBXark/optional-go`: Optional field handling for config migration