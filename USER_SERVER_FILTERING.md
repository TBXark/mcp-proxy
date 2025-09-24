# Per-User Server Access Control

This feature allows you to restrict which MCP servers individual users can access when using OAuth 2.1 authentication. It uses per-server user filters similar to the existing tool filter system.

## Configuration

Add `userFilter` options to individual MCP servers in your `config.json`:

```json
{
  "mcpProxy": {
    "options": {
      "oauth2": {
        "enabled": true,
        "users": {
          "alice": "password123",
          "bob": "password456",
          "admin": "adminpass"
        }
      }
    }
  },
  "mcpServers": {
    "server1": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "options": {
        "userFilter": {
          "mode": "allow",
          "list": ["alice", "admin"]
        }
      }
    },
    "server2": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-git"],
      "options": {
        "userFilter": {
          "mode": "allow",
          "list": ["alice", "bob", "admin"]
        }
      }
    },
    "server3": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-brave-search"],
      "options": {
        "userFilter": {
          "mode": "block",
          "list": ["alice"]
        }
      }
    }
  }
}
```

## User Filter Modes

### Allow Mode (`"mode": "allow"`)
- **Purpose**: Only allow specified users
- **Behavior**: Users must be in the `list` to access the server
- **Example**: `{"mode": "allow", "list": ["alice", "bob"]}` - only alice and bob can access

### Block Mode (`"mode": "block"`)
- **Purpose**: Block specified users, allow all others
- **Behavior**: Users in the `list` are denied access, everyone else is allowed
- **Example**: `{"mode": "block", "list": ["alice"]}` - alice is blocked, others can access

## Example Access Patterns

Based on the configuration above:

- **server1**: Only `alice` and `admin` can access (allow mode)
- **server2**: `alice`, `bob`, and `admin` can access (allow mode)
- **server3**: Everyone except `alice` can access (block mode)

So the effective access is:
- **alice**: Can access `server1` and `server2`, blocked from `server3`
- **bob**: Can access `server2` and `server3`, blocked from `server1`
- **admin**: Can access `server1` and `server2` and `server3`

## How It Works

1. When a user authenticates via OAuth 2.1, their username is stored in the access token
2. Each request includes the username in the request context
3. The `newServerAccessMiddleware` checks the server's `userFilter` configuration
4. If access is denied, the request returns HTTP 403 Forbidden

## Default Behavior

- **No userFilter**: All authenticated users have access (backward compatibility)
- **Empty list**: Behavior depends on mode:
  - Allow mode with empty list: No users allowed
  - Block mode with empty list: All users allowed
- **Token-based authentication**: Bypasses user-specific restrictions (no username available)

## Testing

Use the provided `example_config_with_user_restrictions.json` to test the feature:

1. Start the server: `./build/mcp-proxy --config example_config_with_user_restrictions.json`
2. Authenticate as different users and try accessing different server endpoints
3. Verify that access is properly restricted based on each server's configuration

## Error Messages

When access is denied, users will see:
```
HTTP 403 Forbidden
Access denied: You don't have permission to access this server
```

## Logging

The server logs access decisions with filter details:
```
User alice granted access to server1
User bob denied access to server1 (mode: allow, list: [alice admin])
```

## Benefits of Per-Server Approach

- **Granular Control**: Each server can have different user access rules
- **Flexible Modes**: Use allow-lists for restricted servers, block-lists for open servers
- **Consistent API**: Follows the same pattern as `toolFilter`
- **Independent Configuration**: Server access rules are self-contained
- **Mix and Match**: Some servers can be unrestricted while others have filters