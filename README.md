# MCP Proxy Server

An MCP proxy server that aggregates and serves multiple MCP resource servers through a single HTTP server.

## Features

- **Proxy Multiple MCP Clients**: Connects to multiple MCP resource servers and aggregates their tools and capabilities.
- **SSE Support**: Provides an SSE (Server-Sent Events) server for real-time updates.
- **Flexible Configuration**: Supports multiple client types (`stdio`, `sse` or `streamable-http`) with customizable settings.

## Installation

### Build from Source

 ```bash
git clone https://github.com/TBXark/mcp-proxy.git
cd mcp-proxy
make build
./build/mcp-proxy --config path/to/config.json
```

### Install by go

```bash
go install github.com/TBXark/mcp-proxy@latest
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
    "options": {
      "panicIfInvalid": false,
      "logEnabled": true,
      "authTokens": [
        "DefaultTokens"
      ]
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
        ],
        "toolFilter": {
          "mode": "block", // Must be explicitly set to "allow" or "block"
          "list": ["tool_name_to_block_or_allow"]
        }
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
- `toolFilter`: Optional tool filtering configuration.
  - `mode`: Specifies the filtering mode. Must be explicitly set to "allow" or "block" if `list` is provided. If `list` is present but `mode` is missing or invalid, the filter will be ignored for this server.
  - `list`: A list of tool names to filter (either allow or block based on the `mode`).

  > **Tip:** If you don't know the exact tool names, run the proxy once without any `toolFilter` configured. The console will log messages like `<server_name> Adding tool <tool_name>` for each successfully registered tool. You can use these logged names in your `toolFilter` list.

 > In the new configuration, the `authTokens` of `mcpProxy` is not a global authentication token, but rather the default authentication token for `mcpProxy`. When `authTokens` is set in `mcpServers`, the value of `authTokens` in `mcpServers` will be used instead of the value in `mcpProxy`. In other words, the `authTokens` of `mcpProxy` serves as a default value and is only applied when `authTokens` is not set in `mcpServers`.
> Other fields are the same.

### **`mcpProxy`**
Proxy HTTP server configuration
- `baseURL`: The public accessible URL of the server. This is used to generate the URLs for the clients.
- `addr`: The address the server listens on.
- `name`: The name of the server.
- `version`: The version of the server.
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


## Usage

```
Usage of mcp-proxy:
  -config string
        path to config file or a http(s) url (default "config.json")
  -help
        print help and exit
  -version
        print version and exit
```
1. The server will start and aggregate the tools and capabilities of the configured MCP clients.
2. You can access the server at `http(s)://{baseURL}/{clientName}/sse`. (e.g., `https://mcp.example.com/fetch/sse`, based on the example configuration)
3. If your MCP client does not support custom request headers., you can change the key in `clients` such as `fetch` to `fetch/{authToken}`, and then access it via `fetch/{authToken}`.

## Thanks

- This project was inspired by the [adamwattis/mcp-proxy-server](https://github.com/adamwattis/mcp-proxy-server) project
- If you have any questions about deployment, you can refer to  [《在 Docker 沙箱中运行 MCP Server》](https://miantiao.me/posts/guide-to-running-mcp-server-in-a-sandbox/)([@ccbikai](https://github.com/ccbikai))

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.