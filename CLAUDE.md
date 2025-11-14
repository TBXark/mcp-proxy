# CLAUDE.md - AI Assistant Guide for mcp-proxy

**Last Updated:** 2025-11-14
**Repository:** mcp-proxy - MCP Protocol Aggregation Server
**Language:** Go 1.23
**License:** MIT

---

## Table of Contents

1. [Project Overview](#project-overview)
2. [Repository Structure](#repository-structure)
3. [Key Files and Their Responsibilities](#key-files-and-their-responsibilities)
4. [Architecture and Design Patterns](#architecture-and-design-patterns)
5. [Development Workflow](#development-workflow)
6. [Coding Conventions](#coding-conventions)
7. [Testing Strategy](#testing-strategy)
8. [Common Tasks](#common-tasks)
9. [Important Gotchas](#important-gotchas)
10. [Dependencies](#dependencies)

---

## Project Overview

**mcp-proxy** is an MCP (Model Context Protocol) aggregation server that acts as a reverse proxy to connect multiple MCP resource servers and expose them through a unified HTTP interface.

### Purpose
- Aggregates multiple MCP servers (stdio, SSE, HTTP streaming) into a single endpoint
- Provides flexible tool filtering and authentication per client
- Simplifies MCP client integration for AI assistants like Claude
- Supports concurrent client initialization with graceful error handling

### Key Features
- **Multi-transport Support**: stdio (subprocess), SSE (Server-Sent Events), HTTP streaming
- **Flexible Authentication**: Per-client bearer token validation with inheritance from global defaults
- **Tool Filtering**: Allow/block lists for controlling which tools are exposed
- **Health Monitoring**: Automatic ping tasks for remote clients (SSE/HTTP)
- **Graceful Degradation**: Individual client failures don't crash the entire proxy
- **Config Backward Compatibility**: Automatic V1 → V2 schema migration

### Metrics
- **Total Lines of Code**: ~837 lines (main package only)
- **Files**: 5 Go source files
- **Package Structure**: Monolithic single-package design
- **Test Coverage**: 0% (no automated tests currently)

---

## Repository Structure

```
mcp-proxy/
├── .github/workflows/
│   ├── docker.yml           # Docker image publishing (manual trigger)
│   └── goreleaser.yml       # Binary releases on git tags
├── docs/
│   └── index.html           # Web UI for config conversion (Claude ↔ mcp-proxy)
├── build/                   # Build output directory (gitignored)
├── .golangci.yml            # Linter configuration (strict, formatter-heavy)
├── .goreleaser.yaml         # GoReleaser config (linux amd64/arm64)
├── Dockerfile               # Multi-stage build (Go + Python + Node.js)
├── Makefile                 # Build targets: build, lint, format, buildImage
├── docker-compose.yaml      # Local development setup
├── config.json              # Example configuration
├── go.mod / go.sum          # Dependencies (mcp-go v0.37.0 primary)
├── LICENSE                  # MIT License
├── README.md                # User-facing documentation
└── Source Files:
    ├── main.go              # Entry point, CLI flags, version handling
    ├── client.go            # MCP client lifecycle, tool aggregation, health checks
    ├── config.go            # V2 configuration structs and loading
    ├── config_deprecated.go # V1 config support with auto-migration
    └── http.go              # HTTP server, middleware, routing, shutdown
```

### Important Files to Check First
1. **config.json**: Example configuration showing all features
2. **README.md**: User documentation, configuration reference
3. **client.go**: Core client management logic (342 lines)
4. **http.go**: HTTP server setup and middleware stack (172 lines)

---

## Key Files and Their Responsibilities

### main.go (33 lines)
**Purpose**: Application entry point and CLI interface

**Key Components**:
- CLI flags: `-config`, `-insecure`, `-version`, `-help`
- `BuildVersion` variable (injected via ldflags during build)
- Configuration loading with optional insecure TLS (`-insecure` flag)
- HTTP server startup orchestration

**When to Modify**:
- Adding new CLI flags
- Changing startup behavior
- Version display logic

---

### config.go (180 lines)
**Purpose**: V2 configuration schema and loading logic

**Key Types**:
```go
MCPClientType          // enum: stdio, sse, streamable-http
MCPServerType          // enum: sse, streamable-http
ToolFilterConfig       // mode: allow/block, list: []string
OptionsV2              // panicIfInvalid, logEnabled, authTokens, toolFilter
MCPProxyConfigV2       // Proxy server config (baseURL, addr, type, options)
MCPClientConfigV2      // Individual client config (command/url, options)
ConfigV2               // Root: mcpProxy + mcpServers map
```

**Important Functions**:
- `load(path string, insecure bool)`: Load config from file/HTTP URL
- `parseMCPClientConfigV2()`: Routes config to stdio/SSE/HTTP client creation
- Validation and default value application

**Configuration Hierarchy**:
```
1. Global defaults in OptionsV2
2. mcpProxy.options (server defaults)
3. mcpServers[name].options (per-client overrides)
```

**When to Modify**:
- Adding new configuration fields
- Changing validation logic
- Adding new transport types
- Modifying default values

---

### config_deprecated.go (110 lines)
**Purpose**: V1 configuration backward compatibility

**Key Functions**:
- `parseMCPClientConfigV1()`: Unmarshal legacy config format
- `adaptMCPClientConfigV1ToV2()`: Automatic schema migration

**Important**: Don't remove this file unless explicitly deprecating V1 support. Many existing deployments rely on it.

**When to Modify**:
- Only when V1 migration logic needs fixes
- Eventually can be removed in a major version bump (2.0+)

---

### client.go (342 lines)
**Purpose**: Core MCP client management and aggregation

**Type: Client**
```go
type Client struct {
    name            string         // Unique client identifier
    needPing        bool           // Health check requirement (SSE/HTTP only)
    needManualStart bool           // Requires explicit Start() call
    client          *client.Client // mcp-go client instance
    options         *OptionsV2     // Client-specific options
}
```

**Key Methods**:

1. **`newMCPClient()`** (lines ~30-90)
   - Factory for creating typed MCP clients
   - Routes to stdio/SSE/streamable-http constructors
   - Configures transport-specific options

2. **`addToMCPServer()`** (lines ~100-180)
   - Integrates client with MCP server instance
   - Sends Initialize request (MCP protocol handshake)
   - Aggregates tools, prompts, resources, resource templates
   - Spawns health check goroutine if needed

3. **`startPingTask()`** (lines ~200-250)
   - 30-second interval health checks
   - Failure count tracking
   - Graceful shutdown on context cancellation

4. **Tool Aggregation**:
   - `addToolsToServer()`: Lists tools with pagination, applies filter
   - `addPromptsToServer()`: Registers prompts
   - `addResourcesToServer()`: Static resource registration
   - `addResourceTemplatesToServer()`: Template registration

**Tool Filtering Logic**:
```go
mode: "allow" → Only tools in list are exposed
mode: "block" → Tools in list are filtered out
mode: <invalid> or list: <empty> → All tools exposed
```

**When to Modify**:
- Adding support for new MCP capabilities (sampling, subscriptions)
- Changing health check intervals/logic
- Modifying tool filter behavior
- Adding client lifecycle hooks

**Important Code Locations**:
- Line ~60: stdio client creation with subprocess management
- Line ~80: SSE/HTTP client creation with headers/timeout
- Line ~150: Tool filter application (allow vs block logic)
- Line ~220: Ping task failure handling

---

### http.go (172 lines)
**Purpose**: HTTP server setup, middleware, routing, graceful shutdown

**Type: Server**
```go
type Server struct {
    tokens    []string         // Auth tokens for this server
    mcpServer *server.MCPServer
    handler   http.Handler     // SSE or StreamableHTTP handler
}
```

**Middleware Stack** (applied in order):
1. **`recoverMiddleware()`**: Panic recovery with 500 response + logging
2. **`loggerMiddleware()`**: Request logging (timestamp, method, path)
3. **`newAuthMiddleware(tokens)`**: Bearer token validation (skipped if tokens empty)

**Key Function: `startHTTPServer()`**

**Process Flow**:
```
1. Parse baseURL from config
2. Create HTTP mux and server instance
3. For each configured client:
   a. Create MCP client via newMCPClient()
   b. Create MCP server wrapper via server.NewMCPServer()
   c. Add client to server via addToMCPServer() (concurrent via errgroup)
   d. Register route with middleware chain
   e. Register shutdown cleanup handler
4. Start HTTP listener
5. Handle SIGINT/SIGTERM:
   a. 5-second graceful shutdown timeout
   b. Close all MCP clients
```

**Route Structure**:
- SSE clients: `/{clientName}/sse`
- HTTP streaming clients: `/{clientName}/mcp`
- Optional auth in path: `/{clientName}/{token}/sse` (for clients without header support)

**When to Modify**:
- Adding new middleware (logging, metrics, rate limiting)
- Changing route structure
- Modifying authentication logic
- Adding new HTTP endpoints (health checks, metrics)

**Important Code Locations**:
- Line ~40: Middleware chain construction
- Line ~90: Client initialization error group (concurrent startup)
- Line ~120: Route registration with middleware
- Line ~150: Signal handling and graceful shutdown

---

## Architecture and Design Patterns

### Design Philosophy
**Simplicity over complexity**: Single monolithic binary with minimal external dependencies

### Key Patterns

#### 1. Factory Pattern
**Location**: `client.go:newMCPClient()`
- Routes configuration to appropriate client constructor
- Abstracts transport type differences

#### 2. Middleware Chain
**Location**: `http.go:~40-60`
```go
handler = recoverMiddleware(handler)
handler = loggerMiddleware(handler)
handler = newAuthMiddleware(tokens)(handler)
```
- Composable request processing
- Easy to add new middleware

#### 3. Context-Based Lifecycle
**Usage**: Throughout `client.go` and `http.go`
- All clients receive context for cancellation
- Graceful shutdown via context cancellation
- Timeout management

#### 4. Error Group Concurrency
**Location**: `http.go:~90-110`
```go
g, ctx := errgroup.WithContext(ctx)
for _, c := range clients {
    g.Go(func() error {
        return c.addToMCPServer(ctx, mcpServer)
    })
}
```
- Parallel client initialization
- First error cancels remaining operations

#### 5. Graceful Degradation
**Implementation**: `panicIfInvalid` option
- Individual client failures don't crash server (when false)
- Logged errors with continuation
- Production-friendly error handling

### Concurrency Model

**Goroutines Created**:
1. One per SSE/HTTP client for health checks (`startPingTask()`)
2. Signal listener for SIGINT/SIGTERM
3. Error group goroutines during client initialization

**Synchronization**:
- Context cancellation for shutdown
- No explicit mutexes (minimal shared state)
- HTTP server's built-in request handling concurrency

### Error Handling Strategy

1. **Configuration Errors**: Fail fast at startup
2. **Client Initialization Errors**:
   - If `panicIfInvalid=true`: Log and panic
   - If `panicIfInvalid=false`: Log and continue
3. **Runtime Errors**: Middleware panic recovery + 500 response
4. **Health Check Failures**: Log with failure count, attempt recovery

---

## Development Workflow

### Prerequisites
- **Go**: 1.23 or later
- **golangci-lint**: For code quality checks
- **Docker**: For containerized testing
- **Make**: For build automation

### Common Development Tasks

#### 1. Local Build and Run
```bash
# Build binary
make build

# Run with example config
./build/mcp-proxy --config config.json

# Run with remote config
./build/mcp-proxy --config https://example.com/config.json

# Run with insecure TLS (for testing)
./build/mcp-proxy --config https://localhost/config.json --insecure
```

#### 2. Code Quality
```bash
# Lint code
make lint

# Auto-format code
make format

# Both lint and format before committing
make format && make lint
```

#### 3. Docker Development
```bash
# Start service with docker-compose
docker-compose up

# Access at http://localhost:9090/{clientName}/sse
```

#### 4. Cross-Platform Build
```bash
# Linux x86_64
make buildLinuxX86

# Docker image (amd64 + arm64)
make buildImage
```

### Release Process

#### Creating a Release
```bash
# 1. Commit all changes
git add .
git commit -m "feat: add new feature"

# 2. Tag release
git tag v1.2.3

# 3. Push tag
git push origin v1.2.3

# GoReleaser workflow triggers automatically:
# - Builds linux/amd64 and linux/arm64 binaries
# - Generates changelog from commits
# - Creates GitHub Release with assets
```

#### Manual Docker Image Push
```bash
# Trigger docker.yml workflow manually via GitHub UI
# OR use make target:
make buildImage
```

### Git Workflow

**Branch Strategy**: Main branch development (no feature branches currently)

**Commit Message Convention**: Conventional Commits
```
feat: Add new feature
fix: Bug fix
refactor: Code restructuring
chore: Maintenance tasks
docs: Documentation updates
ci: CI/CD changes

# With scope
fix(http): Correct middleware ordering

# With issue reference
fix: Server crash on invalid config #123
```

**Recent Commits** (as reference):
- `806edea`: chore: update goreleaser workflow
- `cff8330`: fix: improve server type logging #41
- `57e93bd`: chore: bump mcp-go dependency to v0.33.0

---

## Coding Conventions

### Go Style Guide

**Follow**:
- Standard Go conventions (`gofmt`, `goimports`)
- golangci-lint rules in `.golangci.yml`
- Clear variable naming (no abbreviations unless standard)

### File Organization

**Current Structure**: Single package (main)
- All files in repository root
- No subpackages
- Clear separation by concern (client, config, http)

**When Adding New Files**:
- Keep in main package
- Name by primary responsibility (e.g., `metrics.go`, `validation.go`)
- Aim for ~150-200 lines per file

### Naming Conventions

**Types**:
- Exported structs: `MCPClientConfigV2`, `OptionsV2`
- Unexported structs: `Server`, `Client` (used as receiver types)
- Enums: `MCPClientType`, `MCPServerType`

**Functions**:
- Constructors: `newMCPClient()`, `newAuthMiddleware()`
- Handlers: `startHTTPServer()`, `startPingTask()`
- Methods: `addToMCPServer()`, `addToolsToServer()`

**Variables**:
- Descriptive names: `httpServer`, `mcpServer`, `errGroup`
- Single-letter only in tight loops: `for i, c := range clients`

### Error Handling

**Patterns**:
```go
// Check and return
if err != nil {
    return fmt.Errorf("context: %w", err)
}

// Check and log + continue (graceful degradation)
if err != nil {
    slog.Error("operation failed", "error", err)
    if c.options.panicIfInvalid {
        panic(err)
    }
}

// Check and panic (configuration errors)
if err != nil {
    panic(fmt.Sprintf("invalid config: %v", err))
}
```

### Logging

**Usage**:
```go
// Structured logging with slog
slog.Info("server started", "addr", addr)
slog.Error("client failed", "name", clientName, "error", err)

// Conditional logging based on config
if c.options.logEnabled {
    slog.Info("request received", "path", r.URL.Path)
}
```

### Configuration

**Principles**:
1. **Externalized**: All runtime config in JSON
2. **Optional Overrides**: Inherit from proxy defaults, override per-client
3. **Validation**: Fail fast on invalid config at startup
4. **Defaults**: Sensible defaults for optional fields

---

## Testing Strategy

### Current State
- **Unit Tests**: 0 files
- **Integration Tests**: Manual via docker-compose
- **CI Tests**: Build verification only

### Recommended Testing Areas (Not Implemented)

**High Priority**:
1. Configuration parsing and validation
2. V1 → V2 config migration
3. Tool filter logic (allow/block modes)
4. Authentication middleware

**Medium Priority**:
5. Client factory (newMCPClient routing)
6. Middleware chain composition
7. Error handling paths

**Low Priority (Integration)**:
8. End-to-end SSE client aggregation
9. Health check ping/recovery
10. Graceful shutdown behavior

### Manual Testing Workflow

**Example Test Scenario**:
```bash
# 1. Create test config
cat > test-config.json <<EOF
{
  "mcpProxy": {
    "baseURL": "http://localhost:9090",
    "addr": ":9090",
    "type": "sse"
  },
  "mcpServers": {
    "test": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-everything"]
    }
  }
}
EOF

# 2. Run server
./build/mcp-proxy --config test-config.json

# 3. Test endpoint
curl http://localhost:9090/test/sse \
  -H "Accept: text/event-stream"
```

---

## Common Tasks

### Adding a New Configuration Option

**Example**: Add `maxConcurrentRequests` option

1. **Update config.go**:
```go
type OptionsV2 struct {
    // ... existing fields
    MaxConcurrentRequests optional.Int `json:"maxConcurrentRequests,omitempty"`
}
```

2. **Add default value handling** in `parseMCPClientConfigV2()`:
```go
if config.options.MaxConcurrentRequests.IsNil() {
    config.options.MaxConcurrentRequests = optional.NewInt(10)
}
```

3. **Use in http.go** or client.go:
```go
// Example: limit concurrent requests
semaphore := make(chan struct{}, options.MaxConcurrentRequests.Value())
```

4. **Update README.md** with documentation

5. **Update config.json** example

---

### Adding a New Middleware

**Example**: Add request rate limiting

1. **Create middleware in http.go**:
```go
func rateLimitMiddleware(limit int) func(http.Handler) http.Handler {
    limiter := rate.NewLimiter(rate.Limit(limit), burst)
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

2. **Add to middleware chain** in `startHTTPServer()`:
```go
handler = rateLimitMiddleware(100)(handler)
handler = recoverMiddleware(handler)
handler = loggerMiddleware(handler)
```

3. **Test with curl**:
```bash
for i in {1..150}; do curl http://localhost:9090/test/sse & done
# Should see 429 responses after 100 requests
```

---

### Adding Support for a New Transport Type

**Example**: Add WebSocket transport

1. **Update config.go** enums:
```go
type MCPClientType string
const (
    // ... existing
    MCPClientTypeWebSocket MCPClientType = "websocket"
)
```

2. **Add case in client.go:newMCPClient()**:
```go
case MCPClientTypeWebSocket:
    // Create WebSocket client
    wsClient, err := client.NewWebSocketClient(config.URL, ...)
    return &Client{client: wsClient, ...}
```

3. **Update route registration** in http.go:
```go
if clientType == MCPClientTypeWebSocket {
    pattern = fmt.Sprintf("/%s/ws", name)
}
```

4. **Add tests and documentation**

---

### Debugging Client Connection Issues

**Common Issues**:

1. **Client fails to start**:
```bash
# Check logs for:
ERROR client initialization failed name=xyz error=...

# Solution: Check command/args in config
# Verify command exists: which npx, which uvx
```

2. **Authentication failures**:
```bash
# Check logs for:
ERROR unauthorized request

# Solution: Verify Authorization header
curl -H "Authorization: Bearer YOUR_TOKEN" ...
```

3. **Tool not appearing**:
```bash
# Check logs for:
INFO <client_name> Adding tool <tool_name>

# If missing, check toolFilter config
# Try removing filter to see all tools
```

**Debug Mode**:
```json
{
  "mcpServers": {
    "debug-client": {
      "options": {
        "logEnabled": true,
        "panicIfInvalid": true
      }
    }
  }
}
```

---

## Important Gotchas

### 1. Configuration Loading

**Issue**: Config can be loaded from file OR HTTP URL
```bash
./mcp-proxy --config config.json           # Local file
./mcp-proxy --config https://example.com/c.json  # Remote
```

**Gotcha**: Remote config requires valid TLS by default. Use `--insecure` flag only for testing.

**Reference**: `config.go:load()`, line ~160

---

### 2. Tool Filter Mode Validation

**Issue**: Tool filter is ignored if mode is invalid

```json
{
  "toolFilter": {
    "list": ["tool1", "tool2"]
    // MISSING "mode" field → filter is IGNORED
  }
}
```

**Correct**:
```json
{
  "toolFilter": {
    "mode": "allow",  // or "block"
    "list": ["tool1", "tool2"]
  }
}
```

**Reference**: `client.go:addToolsToServer()`, line ~150-170

---

### 3. Authentication Token Inheritance

**Issue**: Tokens can be set at proxy level or per-client

**Behavior**:
```json
{
  "mcpProxy": {
    "options": {
      "authTokens": ["default-token"]  // Default for all clients
    }
  },
  "mcpServers": {
    "client1": {
      // Uses default-token
    },
    "client2": {
      "options": {
        "authTokens": ["specific-token"]  // Overrides default
      }
    }
  }
}
```

**Reference**: `config.go:parseMCPClientConfigV2()`, line ~80-100

---

### 4. Graceful Shutdown Timeout

**Issue**: Server has 5-second timeout for graceful shutdown

**Behavior**:
- SIGINT/SIGTERM received
- Server stops accepting new connections
- Waits up to 5 seconds for active requests
- Force closes after timeout

**Implication**: Long-running SSE connections may be terminated

**Reference**: `http.go:startHTTPServer()`, line ~150-160

---

### 5. Health Check Ping Interval

**Issue**: SSE/HTTP clients are pinged every 30 seconds

**Behavior**:
- Failure count increments on error
- No automatic client restart (only logging)
- Context cancellation stops ping task

**Implication**: Failed clients remain registered but non-functional

**Reference**: `client.go:startPingTask()`, line ~220-240

---

### 6. Concurrent Client Initialization

**Issue**: Clients initialize in parallel via errgroup

**Behavior**:
- First error cancels remaining initializations
- All clients must succeed for server to start (unless `panicIfInvalid=false`)

**Gotcha**: One slow client delays all others

**Reference**: `http.go:startHTTPServer()`, line ~90-110

---

### 7. Build Version Injection

**Issue**: Version is injected at build time via ldflags

**Build Command**:
```makefile
BUILD = $(shell git log -1 --format="%h @ %cd" --date=unix)
LD_FLAGS = -X main.BuildVersion=$(BUILD)
go build -ldflags="$(LD_FLAGS)" -o build/mcp-proxy main.go
```

**Gotcha**: Running `go run main.go` won't set version (shows empty string)

**Reference**: `Makefile`, line ~10-15, `main.go:BuildVersion`

---

### 8. Docker Image Node.js/Python Dependencies

**Issue**: Docker image includes both Node.js and Python runtimes

**Reason**: MCP ecosystem uses both:
- `npx` for JavaScript-based servers (@modelcontextprotocol/*)
- `uvx` for Python-based servers (mcp-server-*)

**Implication**: Image size ~330MB (not alpine-tiny)

**Reference**: `Dockerfile`, line ~15-20

---

### 9. No State Persistence

**Issue**: All state is in-memory; restart loses everything

**Behavior**:
- No database or cache
- Client connections re-established on restart
- No request history or metrics persistence

**Implication**: Not suitable for stateful workflows without external storage

---

### 10. V1 Config Auto-Migration

**Issue**: V1 configs are automatically converted to V2 at runtime

**Behavior**:
- Silent migration (no warning logged)
- Original file not modified
- V1 format remains valid indefinitely

**Gotcha**: Changes to V1 config schema require updating migration logic

**Reference**: `config_deprecated.go:adaptMCPClientConfigV1ToV2()`

---

## Dependencies

### Direct Dependencies

#### mcp-go (v0.37.0)
- **Purpose**: MCP protocol implementation
- **Repository**: github.com/mark3labs/mcp-go
- **Usage**: Client/Server interfaces, transport types
- **Key Types**: `client.Client`, `server.MCPServer`
- **Version Strategy**: Manually updated (check for updates regularly)

#### confstore (v0.0.5)
- **Purpose**: Configuration loading from files/URLs
- **Repository**: github.com/TBXark/confstore
- **Usage**: `config.go:load()`
- **Features**: HTTP/HTTPS support, custom HTTP client

#### optional-go (v0.0.1)
- **Purpose**: Optional field wrapper for JSON
- **Repository**: github.com/TBXark/optional-go
- **Usage**: Boolean fields in OptionsV2 (enables true omitempty)
- **Pattern**: `optional.Bool`, `optional.Int`

### Key Transitive Dependencies

- `golang.org/x/sync/errgroup`: Concurrent goroutine error handling
- `github.com/google/uuid`: UUID generation (MCP protocol)
- `gopkg.in/yaml.v3`: YAML support (confstore)

### Updating Dependencies

```bash
# Check for updates
go list -u -m all

# Update specific dependency
go get github.com/mark3labs/mcp-go@latest

# Update all dependencies
go get -u ./...

# Tidy and verify
go mod tidy
go mod verify

# Test build
make build

# Commit
git add go.mod go.sum
git commit -m "chore: update dependencies"
```

---

## Additional Resources

### Documentation
- **README.md**: User-facing documentation and configuration reference
- **docs/index.html**: Web UI for config conversion (Claude ↔ mcp-proxy)
- **config.json**: Example configuration with all features

### External References
- **MCP Protocol**: https://modelcontextprotocol.io
- **mcp-go Library**: https://github.com/mark3labs/mcp-go
- **Inspiration**: https://github.com/adamwattis/mcp-proxy-server

### CI/CD
- **Docker Images**: ghcr.io/tbxark/mcp-proxy:latest
- **GitHub Releases**: https://github.com/TBXark/mcp-proxy/releases
- **Workflow Files**: `.github/workflows/docker.yml`, `.github/workflows/goreleaser.yml`

---

## Contact and Contributing

**Repository**: https://github.com/TBXark/mcp-proxy
**License**: MIT
**Issues**: https://github.com/TBXark/mcp-proxy/issues

### Contributing Guidelines (Inferred)
1. Follow existing code style (run `make format`)
2. Lint before committing (run `make lint`)
3. Use conventional commit messages
4. Test locally with docker-compose
5. Update README.md for user-facing changes
6. Update this CLAUDE.md for architectural changes

---

## Changelog

### 2025-11-14 - Initial CLAUDE.md Creation
- Comprehensive codebase analysis
- Documented all 5 source files
- Added common tasks and gotchas
- Established conventions for future AI assistance

---

**End of CLAUDE.md**
