# MCP Proxy Usage Guide

This document explains how to connect to and use the MCP Proxy server.

## Available Endpoints

When the MCP Proxy server starts, it will expose several endpoints:

1. **SSE Endpoints**: These are the main connection points for MCP clients.
2. **Message Endpoints**: Used internally by the SSE connections.
3. **Health Check**: A simple endpoint to check if the server is running.
4. **Metrics**: Prometheus metrics endpoint (if enabled).
5. **Paths**: Lists all available API paths.

## Connecting to MCP Services

To connect to an MCP service through the proxy, you need to connect to the SSE endpoint for that service.

### Finding Available Services

You can list all available services and their endpoints using the `/paths` endpoint:

```bash
curl http://localhost:8080/paths
```

This will return a JSON object listing all available endpoints:

```json
{
  "/example-filesystem-mcp-code/sse": "SSE connection endpoint for example-filesystem-mcp-code MCP service",
  "/example-filesystem-mcp-code/message": "Message endpoint for example-filesystem-mcp-code MCP service (used internally)",
  "/health": "Health check endpoint",
  "/metrics": "Prometheus metrics endpoint",
  "/paths": "List of available API paths"
}
```

### Connecting to a Service Using SSE

The correct way to connect to an MCP service is through its SSE endpoint. For example, to connect to the "example-filesystem-mcp-code" service:

```bash
curl -N http://localhost:8080/example-filesystem-mcp-code/sse
```

This will establish a Server-Sent Events (SSE) connection to the service.

### Using MCP Clients

Most users will want to use an MCP client library rather than connecting directly. When configuring your MCP client, set the endpoint to the SSE endpoint of the service you want to connect to.

Example with JavaScript client:

```javascript
import { connectMCP } from '@modelcontextprotocol/client';

const client = await connectMCP({
  endpoint: 'http://localhost:8080/example-filesystem-mcp-code/sse'
});

// Now you can use the client to call tools
const result = await client.callTool('read_file', { path: '/path/to/file' });
console.log(result);
```

## Using the Health Check Endpoint

To check if the server is running:

```bash
curl http://localhost:8080/health
```

This should return `OK` if the server is healthy.

## Common Issues

### 404 Not Found When Connecting

If you're getting a 404 error when trying to connect to a service, make sure you're using the correct endpoint path. The SSE endpoint is the one ending with `/sse`, not just the base path.

Incorrect:
```bash
curl http://localhost:8080/example-filesystem-mcp-code
```

Correct:
```bash
curl http://localhost:8080/example-filesystem-mcp-code/sse
```

### Connection Closed Unexpectedly

SSE connections require proper headers to be maintained. When using curl, make sure to use the `-N` flag to disable buffering:

```bash
curl -N http://localhost:8080/example-filesystem-mcp-code/sse
```

## Programmatic API Example

Here's a complete example of how to use the MCP Proxy with the JavaScript client:

```javascript
import { connectMCP } from '@modelcontextprotocol/client';

async function main() {
  try {
    // Connect to the MCP service through the proxy
    const client = await connectMCP({
      endpoint: 'http://localhost:8080/example-filesystem-mcp-code/sse'
    });
    
    // List available tools
    const tools = await client.listTools();
    console.log('Available tools:', tools);
    
    // Call a tool
    if (tools.includes('read_file')) {
      const result = await client.callTool('read_file', { path: '/tmp/example.txt' });
      console.log('File contents:', result);
    }
    
    // Close the connection when done
    await client.close();
  } catch (error) {
    console.error('Error:', error);
  }
}

main();
```

## Working with MCP in Python

For Python users, the appropriate way to connect would be:

```python
from mcp_client import MCPClient

# Connect to the MCP service through the proxy
client = MCPClient(endpoint="http://localhost:8080/example-filesystem-mcp-code/sse")

# Now you can use the client to call tools
result = client.call_tool("read_file", {"path": "/tmp/example.txt"})
print(result)
```

## Using the Metrics Endpoint

If metrics are enabled, you can access Prometheus metrics at:

```bash
curl http://localhost:8080/metrics
```

This will return metrics in Prometheus format that can be scraped by a Prometheus server.