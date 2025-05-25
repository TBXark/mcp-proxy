# MCP Proxy Configuration Examples

This directory contains various configuration examples to help you get started with MCP Proxy. Each example demonstrates different use cases and configuration patterns.

## 📁 Configuration Files

### 1. `simple-example.json`
A minimal configuration to get started quickly with just two MCP servers.

**Use case**: First-time users who want to test the proxy with basic functionality.

**Included servers**:
- Memory server (knowledge graph)
- Filesystem server (safe file access)

### 2. `comprehensive-example.json`
A complete configuration showcasing all major MCP server types and features.

**Use case**: Power users who want to see all available options and configuration patterns.

**Included servers**:
- Brave Search (web search)
- GitHub (repository management)
- Filesystem (file operations)
- Memory (knowledge graph)
- Puppeteer (browser automation)
- Git (repository operations)
- Sequential Thinking (reasoning)
- Remote SSE server example
- Streamable HTTP server example

**Features demonstrated**:
- Tool filtering with allow/block lists
- Environment variable configuration
- Authentication tokens
- Remote server connections
- Multiple transport types

### 3. `claude-desktop-config.json`
Configuration file for Claude Desktop to connect to the MCP Proxy.

**Use case**: Replace your complex Claude Desktop configuration with a single proxy connection.

## 🚀 Quick Start

1. **Choose a configuration** that matches your needs
2. **Copy and customize** the configuration file:
   ```bash
   cp examples/configs/simple-example.json config.json
   ```
3. **Update the configuration** with your actual:
   - API keys and tokens
   - File paths
   - Server URLs
4. **Start the proxy**:
   ```bash
   ./build/mcp-proxy --config config.json
   ```

## 🔧 Configuration Guide

### Basic Structure

Every MCP Proxy configuration has two main sections:

```json
{
  "mcpProxy": {
    // Proxy server settings
  },
  "mcpServers": {
    // Individual MCP servers to proxy
  }
}
```

### Proxy Settings (`mcpProxy`)

| Field | Description | Required |
|-------|-------------|----------|
| `baseURL` | Public URL of the proxy server | Yes |
| `addr` | Address to listen on (e.g., `:9090`) | Yes |
| `name` | Display name for the proxy | No |
| `version` | Version identifier | No |
| `options.logEnabled` | Enable detailed logging | No |
| `options.authTokens` | Array of valid auth tokens | No |

### Server Configuration (`mcpServers`)

Each server in `mcpServers` can be configured as:

#### Stdio Servers (Command-line)
```json
"server-name": {
  "command": "npx",
  "args": ["-y", "package-name"],
  "env": {
    "API_KEY": "your-key"
  }
}
```

#### SSE Servers (Remote)
```json
"server-name": {
  "url": "https://server.com/sse",
  "headers": {
    "Authorization": "Bearer token"
  }
}
```

#### Streamable HTTP Servers
```json
"server-name": {
  "transportType": "streamable-http",
  "url": "https://server.com/mcp",
  "timeout": 30
}
```

### Tool Filtering

Control which tools are available from each server:

```json
"options": {
  "toolFilter": {
    "mode": "allow",  // or "block"
    "list": ["tool1", "tool2", "tool3"]
  }
}
```

**Modes**:
- `allow`: Only listed tools are available
- `block`: Listed tools are blocked, others are available

## 🔒 Security Best Practices

1. **Use tool filtering** to limit available functionality
2. **Set authentication tokens** for proxy access
3. **Limit filesystem paths** to safe directories only
4. **Use environment variables** for sensitive data
5. **Review server permissions** regularly

## 🛠️ Environment Variables

For security, use environment variables for sensitive data:

```bash
export GITHUB_TOKEN="your-token"
export BRAVE_API_KEY="your-key"
```

Then reference in config:
```json
"env": {
  "GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}"
}
```

## 📋 Common Server Examples

### Popular MCP Servers

| Server | Package | Description |
|--------|---------|-------------|
| **Memory** | `@modelcontextprotocol/server-memory` | Knowledge graph storage |
| **Filesystem** | `@modelcontextprotocol/server-filesystem` | File operations |
| **GitHub** | `@modelcontextprotocol/server-github` | Repository management |
| **Brave Search** | `@modelcontextprotocol/server-brave-search` | Web search |
| **Puppeteer** | `@modelcontextprotocol/server-puppeteer` | Browser automation |
| **Git** | `mcp-server-git` (via uvx) | Git operations |

### Installation Commands

```bash
# Install common MCP servers
npm install -g @modelcontextprotocol/server-memory
npm install -g @modelcontextprotocol/server-filesystem
npm install -g @modelcontextprotocol/server-github
npm install -g @modelcontextprotocol/server-brave-search
npm install -g @modelcontextprotocol/server-puppeteer

# Install Python-based servers (choose one method)
# Using uv (recommended):
uvx mcp-server-git

# Or using pip:
pip install mcp-server-git
```

## 🔍 Debugging

1. **Enable logging** in proxy configuration:
   ```json
   "options": {
     "logEnabled": true
   }
   ```

2. **Check server connections** in logs
3. **Verify tool registration** messages
4. **Test individual servers** before adding to proxy

## 🤝 Contributing

Found a useful configuration pattern? Please contribute!

1. Add your example to `examples/configs/`
2. Update this README with documentation
3. Submit a pull request

## 📚 Additional Resources

- [MCP Specification](https://modelcontextprotocol.io/)
- [Official MCP Servers](https://github.com/modelcontextprotocol/servers)
- [Claude Desktop Integration](https://docs.anthropic.com/claude/docs)
