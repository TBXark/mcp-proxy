# MCP Proxy Server

An MCP proxy server that aggregates and serves multiple MCP resource servers through a single HTTP server.

## Features

- **Proxy Multiple MCP Clients**: Connects to multiple MCP resource servers and aggregates their tools and capabilities.
- **SSE / HTTP Streaming MCPSupport**: Provides an SSE (Server-Sent Events) or HTTP streaming interface for real-time updates from MCP clients.
- **Flexible Configuration**: Supports multiple client types (`stdio`, `sse` or `streamable-http`) with customizable settings.

## Installation

### Build from Source

 ```bash
git clone https://github.com/TBXark/mcp-proxy.git
cd mcp-proxy
make build
./build/mcp-proxy --config path/to/config.json
```

**Note**: OAuth templates are built-in by default. Use `--eject-templates` to customize them.

### Install by go

```bash
# Install the latest version of mcp-proxy
go install github.com/TBXark/mcp-proxy@latest
# Or install stable version
go install github.com/TBXark/mcp-proxy
````

### Docker

> The Docker image supports two MCP calling methods by default: `npx` and `uvx`.

```bash
docker run -d -p 9090:9090 -v /path/to/config.json:/config/config.json ghcr.io/tbxark/mcp-proxy:latest
# or 
docker run -d -p 9090:9090 ghcr.io/tbxark/mcp-proxy:latest --config https://example.com/path/to/config.json
```

## Configuration

The server is configured using a JSON file. Below is an example configuration:

> This is the format for the new version's configuration. The old version's configuration will be automatically converted to the new format's configuration when it is loaded.
> You can use [`https://tbxark.github.io/mcp-proxy`](https://tbxark.github.io/mcp-proxy) to convert the configuration of `mcp-proxy` into the configuration that `Claude` can use.

```jsonc
{
  "mcpProxy": {
    "baseURL": "https://mcp.example.com",
    "addr": ":9090",
    "name": "MCP Proxy",
    "version": "1.0.0",
    "type": "streamable-http",// The transport type of the MCP proxy server, can be `streamable-http`, `sse`. By default, it is `sse`.
    "options": {
      "panicIfInvalid": false,
      "logEnabled": true,
      "authTokens": [
        "DefaultTokens"
      ],
      "oauth2": {
        "enabled": true,
        "users": {
          "admin": "password123",
          "user": "mypassword"
        },
        "persistenceDir": "/custom/path/oauth",
        "allowedIPs": [
          "34.162.46.92",
          "34.162.102.82",
          "34.162.136.91", 
          "34.162.142.92",
          "34.162.183.95"
        ],
        "templateDir": "/custom/templates"
      }
    }
  },
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-github"
      ],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "<YOUR_TOKEN>"
      },
      "options": {
        "toolFilter": {
          "mode": "block",
          "list": [
            "create_or_update_file"
          ]
        }
      }
    },
    "fetch": {
      "command": "uvx",
      "args": [
        "mcp-server-fetch"
      ],
      "options": {
        "panicIfInvalid": true,
        "logEnabled": false,
        "authTokens": [
          "SpecificTokens"
        ]
      }
    },
    "amap": {
      "url": "https://mcp.amap.com/sse?key=<YOUR_TOKEN>"
    }
  }
}
```

### **`options`**

Common options for `mcpProxy` and `mcpServers`.

- `panicIfInvalid`: If true, the server will panic if the client is invalid.
- `logEnabled`: If true, the server will log the client's requests.
- `authTokens`: A list of authentication tokens for the client. The `Authorization` header will be checked against this list.
- `oauth2`: OAuth 2.1 Authorization Server configuration. **Only applies when proxy type is `streamable-http`.**
  - `enabled`: Enable/disable the OAuth 2.1 server. Set to `true` for Claude Desktop integration.
  - `users`: Username/password pairs for authentication. Users must provide valid credentials to authorize access.
  - `persistenceDir`: Directory for storing OAuth client registrations. Defaults to `$HOME/.mcpproxy` if not specified.
  - `allowedIPs`: IP addresses permitted to register OAuth clients. Use Claude's official IPs for security. Empty array allows all IPs.
  - `tokenExpirationMinutes`: Access token expiration time in minutes. Defaults to 60 minutes (1 hour) if not specified.
  - `templateDir`: Base directory for OAuth HTML templates. Server looks for templates in `{templateDir}/oauth/`. Defaults to `templates` if not specified.
- `toolFilter`: Optional tool filtering configuration. **This configuration is only effective in `mcpServers`.**
  - `mode`: Specifies the filtering mode. Must be explicitly set to `allow` or `block` if `list` is provided. If `list` is present but `mode` is missing or invalid, the filter will be ignored for this server.
  - `list`: A list of tool names to filter (either allow or block based on the `mode`).
  > **Tip:** If you don't know the exact tool names, run the proxy once without any `toolFilter` configured. The console will log messages like `<server_name> Adding tool <tool_name>` for each successfully registered tool. You can use these logged names in your `toolFilter` list.

> In the new configuration, the `authTokens` of `mcpProxy` is not a global authentication token, but rather the default authentication token for `mcpProxy`. When `authTokens` is set in `mcpServers`, the value of `authTokens` in `mcpServers` will be used instead of the value in `mcpProxy`. In other words, the `authTokens` of `mcpProxy` serves as a default value and is only applied when `authTokens` is not set in `mcpServers`. Other fields are the same.

### **`mcpProxy`**

Proxy HTTP server configuration

- `baseURL`: The public accessible URL of the server. This is used to generate the URLs for the clients.
- `addr`: The address the server listens on.
- `name`: The name of the server.
- `version`: The version of the server.
- `type`: The transport type of the MCP proxy server. Can be `streamable-http` or `sse`. By default, it is `sse`.
  - `streamable-http`: The MCP proxy server supports HTTP streaming.
  - `sse`: The MCP proxy server supports Server-Sent Events (SSE).
- `options`: Default options for the `mcpServers`.

### **`mcpServers`**

MCP server configuration, Adopt the same configuration format as other MCP Clients.

- `transportType`: The transport type of the MCP client. Except for `streamable-http`, which requires manual configuration, the rest will be automatically configured according to the content of the configuration file.
  - `stdio`: The MCP client is a command line tool that is run in a subprocess.
  - `sse`: The MCP client is a server that supports SSE (Server-Sent Events).
  - `streamable-http`: The MCP client is a server that supports HTTP streaming.

For stdio mcp servers, the `command` field is required.

- `command`: The command to run the MCP client.
- `args`: The arguments to pass to the command.
- `env`: The environment variables to set for the command.
- `options`: Options specific to the client.

For sse mcp servers, the `url` field is required. When the current `url` exists, `sse` will be automatically configured.

- `url`: The URL of the MCP client.
- `headers`: The headers to send with the request to the MCP client.

For http streaming mcp servers, the `url` field is required. and `transportType` need to manually set to `streamable-http`.

- `url`: The URL of the MCP client.
- `headers`: The headers to send with the request to the MCP client.
- `timeout`: The timeout for the request to the MCP client.

### OAuth 2.1 Authorization Server

When using `streamable-http` transport, the proxy acts as a complete OAuth 2.1 Authorization Server designed for Claude Desktop integration. This provides secure, standards-compliant authentication with advanced security features.

```jsonc
{
  "mcpProxy": {
    "baseURL": "https://mcp.example.com",
    "addr": ":9090",
    "name": "MCP Proxy",
    "version": "1.0.0",
    "type": "streamable-http",
    "options": {
      "oauth2": {
        "enabled": true,
        "users": {
          "admin": "password123",
          "user": "mypassword"
        },
        "persistenceDir": "/custom/path/for/oauth",
        "allowedIPs": [
          "34.162.46.92",
          "34.162.102.82",
          "34.162.136.91",
          "34.162.142.92",
          "34.162.183.95"
        ]
      }
    }
  },
  "mcpServers": {
    "neo4j-memory": {
      "command": "docker",
      "args": ["run", "-i", "--rm", "mcp/neo4j-memory"]
    }
  }
}
```

#### OAuth Flow Features

- **🔐 RFC 7591 Dynamic Client Registration**: Claude Desktop automatically registers without manual setup
- **🛡️ PKCE Support**: Proof Key for Code Exchange prevents authorization code interception attacks
- **👤 Username/Password Authentication**: Secure login form validates against configured user credentials
- **🎫 Bearer Token Authorization**: All MCP endpoints require valid OAuth access tokens
- **💾 Token Persistence**: Clients, access tokens, and refresh tokens survive server restarts
- **🔄 Refresh Token Support**: Automatic token renewal for seamless long-term access
- **🌐 IP Allowlisting**: Restrict client registration to Claude's official IP addresses
- **🔒 Callback URL Validation**: Only official Claude callback URLs are accepted

#### OAuth Endpoints

The proxy automatically exposes these OAuth endpoints:

- `GET /.well-known/oauth-authorization-server` - Server metadata discovery
- `POST /oauth/register` - Dynamic client registration
- `GET /oauth/authorize` - Authorization endpoint with login form  
- `POST /oauth/token` - Token exchange endpoint

#### Persistence Directory

OAuth data (clients, access tokens, refresh tokens) is persisted across server restarts. Default location is `$HOME/.mcpproxy/oauth_clients.json`. 

You can customize the persistence directory:

```jsonc
{
  "mcpProxy": {
    "options": {
      "oauth2": {
        "enabled": true,
        "users": {
          "admin": "password123"
        },
        "persistenceDir": "/var/lib/mcpproxy"
      }
    }
  }
}
```

#### IP Allowlisting

You can restrict OAuth client registration to specific IP addresses for enhanced security:

```jsonc
{
  "mcpProxy": {
    "options": {
      "oauth2": {
        "enabled": true,
        "users": {
          "admin": "password123"
        },
        "allowedIPs": [
          "34.162.46.92",
          "34.162.102.82", 
          "34.162.136.91",
          "34.162.142.92",
          "34.162.183.95"
        ]
      }
    }
  }
}
```

**Note**: The IP addresses above are Claude's official IP addresses as documented at https://docs.anthropic.com/en/api/ip-addresses#ipv4-2. Using this allowlist ensures only Claude Desktop can register OAuth clients with your proxy.

**Proxy Support**: The IP detection works correctly with various proxy configurations:
- **Cloudflare**: `CF-Connecting-IP`, `True-Client-IP`
- **nginx**: `X-Real-IP`, `X-Forwarded-For`  
- **AWS ALB/ELB**: `X-Forwarded-For`
- **Kubernetes Ingress**: `X-Cluster-Client-IP`
- **RFC 7239 Standard**: `Forwarded` header
- **ngrok/tunnels**: `X-Forwarded-For`
- **Direct connections**: `RemoteAddr`

#### Configuration Examples

**Minimal OAuth Setup (Development)**:
```jsonc
{
  "mcpProxy": {
    "type": "streamable-http",
    "options": {
      "oauth2": {
        "enabled": true,
        "users": {
          "developer": "dev-password"
        }
      }
    }
  }
}
```

**Production OAuth Setup (Recommended)**:
```jsonc
{
  "mcpProxy": {
    "type": "streamable-http", 
    "options": {
      "oauth2": {
        "enabled": true,
        "users": {
          "admin": "secure-admin-password",
          "user": "secure-user-password"
        },
        "persistenceDir": "/var/lib/mcpproxy/oauth",
        "allowedIPs": [
          "34.162.46.92",
          "34.162.102.82",
          "34.162.136.91",
          "34.162.142.92", 
          "34.162.183.95"
        ]
      }
    }
  }
}
```

**Development/Testing Setup (No IP Restrictions)**:
```jsonc
{
  "mcpProxy": {
    "type": "streamable-http",
    "options": {
      "oauth2": {
        "enabled": true,
        "users": {
          "test": "test123"
        },
        "allowedIPs": [],
        "tokenExpirationMinutes": 60
      }
    }
  }
}
```

#### Security Features

- **🔐 Username/Password Authentication**: All OAuth flows require valid user credentials
- **🎫 Bearer Token Access**: MCP endpoints require `Authorization: Bearer <token>` header
- **🔑 Client Secret Validation**: Generated client secrets are cryptographically validated
- **📁 Secure Persistence**: OAuth data (clients + tokens) stored with 0700 permissions (owner-only)
- **🌐 IP Allowlisting**: Optional restriction to Claude's official IP addresses
- **🔒 Callback URL Validation**: Only official Claude URLs accepted as redirect targets
- **🔄 Configurable Token Expiration**: Access tokens expire after configurable time (default: 1 hour)
- **♻️ Refresh Token Rotation**: New refresh token issued on each refresh (OAuth 2.1 best practice)

#### Claude Desktop Setup

Once your proxy is running with OAuth enabled, configure Claude Desktop:

1. **Add MCP Server**: In Claude Desktop settings, add a new MCP server
2. **Server URL**: Use your proxy's base URL (e.g., `https://your-domain.com` or `https://your-tunnel.ngrok.io`)
3. **Authentication**: Claude Desktop will automatically:
   - Discover the OAuth endpoints via `.well-known/oauth-authorization-server`
   - Register as an OAuth client via Dynamic Client Registration
   - Present a login form for username/password authentication
   - Handle token refresh automatically

**Example Claude Desktop MCP Configuration**:
```json
{
  "mcpServers": {
    "your-proxy": {
      "command": "mcp",
      "args": ["--server", "https://your-domain.com/your-mcp-server"]
    }
  }
}
```

### OAuth Template Customization

The OAuth 2.1 server includes built-in HTML templates for the authorization and success pages, with support for customization.

#### Built-in Templates

By default, the OAuth templates are embedded in the binary and require no external files. The server automatically uses these built-in templates.

#### Ejecting Templates for Customization

To customize the OAuth pages, first eject the templates:

```bash
./mcp-proxy --eject-templates
```

This creates:
```
templates/
└── oauth/
    ├── authorize.html    # Login form page
    └── success.html      # Success/redirect page
```

#### Template Override Behavior

The server loads templates in this priority order:
1. **External templates**: `{templateDir}/oauth/*.html` (where `templateDir` is from config, default: `templates`)
2. **Built-in templates**: Embedded defaults if external templates don't exist or fail to load
3. **To revert to built-in**: Remove the template directory or set `templateDir` to a non-existent path

#### Hot Reloading

When using external templates, the server automatically detects file changes and reloads templates **without requiring a restart**:

- **Automatic detection**: Checks file modification times on every OAuth request
- **Zero-config**: Hot reloading is always enabled for external templates
- **Live development**: Edit templates and see changes immediately in your browser
- **Fallback protection**: If reloading fails, continues using the previous templates

This makes template customization seamless during development and testing.

#### Template Data

**authorize.html** receives:
- `ClientName` - Application name (usually "Claude")
- `ResourceName` - Resource being accessed  
- `ClientID`, `RedirectURI`, `ResponseType`, `Scope`, `State`, `CodeChallenge`, `Resource` - OAuth parameters
- `ErrorMessage` - Error message to display (if any)

**success.html** receives:
- `Username` - Authenticated user's username
- `RedirectURL` - Complete redirect URL with authorization code

#### Customizing Templates

After ejecting templates:

1. Edit the HTML files in `templates/oauth/`
2. Maintain the form structure and hidden fields in `authorize.html`
3. Keep the JavaScript redirect functionality in `success.html`
4. Restart the proxy server to reload templates

The templates use Go's `html/template` package with automatic XSS protection and context-aware escaping.

## Usage

```bash
Usage of mcp-proxy:
  -config string
        path to config file or a http(s) url (default "config.json")
  -eject-templates
        eject OAuth templates to configured templateDir/oauth/ (or templates/oauth/ if not configured)
  -eject-templates-to string
        eject OAuth templates to specified directory (overrides config templateDir)
  -help
        print help and exit
  -insecure
        allow insecure HTTPS connections by skipping TLS certificate verification
  -version
        print version and exit
```

1. The server will start and aggregate the tools and capabilities of the configured MCP clients.
2. When MCP Server type is `sse`, You can access the server at `http(s)://{baseURL}/{clientName}/sse`. (e.g., `https://mcp.example.com/fetch/sse`, based on the example configuration)
3. When MCP Server type is `streamable-http`, You can access the server at `http(s)://{baseURL}/{clientName}/mcp`. (e.g., `https://mcp.example.com/fetch/mcp`, based on the example configuration)
4. If your MCP client does not support custom request headers., you can change the key in `clients` such as `fetch` to `fetch/{authToken}`, and then access it via `fetch/{authToken}`.

## Thanks

- This project was inspired by the [adamwattis/mcp-proxy-server](https://github.com/adamwattis/mcp-proxy-server) project
- If you have any questions about deployment, you can refer to  [《在 Docker 沙箱中运行 MCP Server》](https://miantiao.me/posts/guide-to-running-mcp-server-in-a-sandbox/)([@ccbikai](https://github.com/ccbikai))

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
