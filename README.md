# MCP Proxy Setup

A centralized MCP (Model Context Protocol) proxy server configuration for managing multiple MCP servers through a single interface. This setup simplifies Claude Desktop configuration by aggregating all MCP servers behind one proxy.

## 🎯 Purpose

Instead of managing 15+ individual MCP server connections in Claude Desktop, this proxy allows you to:
- Connect to **one proxy server** that handles everything
- Centralize configuration and logging
- Implement security controls and tool filtering
- Easily add/remove MCP servers without changing Claude config

## 🏗️ Architecture

```
Claude Desktop → MCP Proxy → Your Individual MCP Servers
                     ↓
                [brave-search, github, filesystem, puppeteer, 
                 memory, sequential-thinking, git, browser-tools, etc.]
```

## 🚀 Quick Start

### 1. Prerequisites

- Go 1.19+ (for building the proxy)
- Node.js and npm (for most MCP servers)
- uv/uvx (for Python-based MCP servers)

### 2. Clone and Build

```bash
git clone https://github.com/jlwainwright/mcp-proxy-setup.git
cd mcp-proxy-setup
make build
```

### 3. Configure

```bash
# Copy example configuration
cp proxy-config.example.json proxy-config.json
cp claude-config.example.json claude-config.json

# Edit proxy-config.json with your actual:
# - API keys
# - File paths  
# - Environment variables
```

### 4. Update Claude Desktop

Replace your Claude Desktop configuration:

**On macOS:**
```bash
# Backup current config
cp "~/Library/Application Support/Claude/claude_desktop_config.json" \
   "~/Library/Application Support/Claude/claude_desktop_config.json.backup"

# Update with proxy config (edit paths first!)
cp claude-config.json "~/Library/Application Support/Claude/claude_desktop_config.json"
```

**On Windows:**
```bash
# Backup current config
cp "%APPDATA%/Claude/claude_desktop_config.json" \
   "%APPDATA%/Claude/claude_desktop_config.json.backup"

# Update with proxy config (edit paths first!)
cp claude-config.json "%APPDATA%/Claude/claude_desktop_config.json"
```

### 5. Start the Proxy

```bash
./build/mcp-proxy --config config.json
```

### 6. Restart Claude Desktop

## 📋 Included MCP Servers

This configuration includes popular MCP servers:

| Server | Description | Tools |
|--------|-------------|-------|
| **brave-search** | Web search capabilities | Web search, local search |
| **github** | GitHub repository management | Repo operations, code search |
| **filesystem** | Safe file system access | File operations, directory listing |
| **puppeteer** | Browser automation | Screenshots, navigation, interaction |
| **memory** | Knowledge graph storage | Entity management, relations |
| **sequential-thinking** | Step-by-step reasoning | Structured thinking |
| **git** | Git repository operations | Status, commit, branch management |
| **iterm** | Terminal integration | Command execution |
| **obsidian** | Note management | Vault operations |
| **browser-tools** | Browser dev tools | Debugging, auditing |

## 🔧 Configuration

### Proxy Settings (`proxy-config.json`)

```json
{
  "mcpProxy": {
    "baseURL": "http://localhost:9090",
    "addr": ":9090",
    "name": "MCP Proxy",
    "options": {
      "logEnabled": true,
      "authTokens": ["your-auth-token"]
    }
  },
  "mcpServers": {
    "server-name": {
      "command": "npx",
      "args": ["-y", "package-name"],
      "env": {
        "API_KEY": "your-api-key"
      },
      "options": {
        "toolFilter": {
          "mode": "allow",
          "list": ["tool1", "tool2"]
        }
      }
    }
  }
}
```

### Claude Desktop Config (`claude-config.json`)

```json
{
  "mcpServers": {
    "mcp-proxy": {
      "command": "/path/to/mcp-proxy/build/mcp-proxy",
      "args": ["--config", "/path/to/proxy-config.json"]
    }
  }
}
```

## 🔒 Security Features

- **Tool Filtering**: Each server has explicit allow-lists
- **Authentication**: Auth tokens for proxy access  
- **Path Restrictions**: Filesystem access limited to safe directories
- **Environment Isolation**: Each MCP server runs separately

## 🛠️ Management

### Adding New Servers

1. Add to `proxy-config.json`:
```json
"new-server": {
  "command": "npx",
  "args": ["-y", "new-mcp-package"],
  "options": {
    "toolFilter": {
      "mode": "allow", 
      "list": ["allowed-tool"]
    }
  }
}
```

2. Restart the proxy

### Removing Servers

1. Delete the server entry from `proxy-config.json`
2. Restart the proxy

### Viewing Logs

The proxy provides detailed logs showing:
- Server connection status
- Tool registrations
- Request routing
- Error details

## 🚨 Troubleshooting

### Common Issues

**Proxy won't start**
- Check if port 9090 is available: `lsof -i :9090`
- Verify all MCP packages are installed
- Check file paths in configuration

**Claude can't connect**
- Ensure proxy is running
- Verify claude-config.json paths are correct
- Restart Claude Desktop after config changes

**Missing tools**
- Check proxy logs for "Adding tool" messages
- Verify tool names in filter lists match exactly
- Ensure API keys are valid

## 📚 Resources

- [Model Context Protocol Specification](https://modelcontextprotocol.io/)
- [Official MCP Servers](https://github.com/modelcontextprotocol/servers)
- [TBXark MCP Proxy](https://github.com/TBXark/mcp-proxy) (base proxy)

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test with your setup
5. Submit a pull request

## 📝 License

This project follows the MIT License. See individual MCP server licenses for their terms.

## 🙏 Acknowledgments

- [TBXark](https://github.com/TBXark) for the excellent mcp-proxy implementation
- [Anthropic](https://github.com/anthropic) for Claude and MCP specification
- The MCP community for creating amazing servers

---

**Need help?** Open an issue or check the troubleshooting section above.
