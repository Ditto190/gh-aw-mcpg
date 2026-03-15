# MCP Gateway

A gateway for Model Context Protocol (MCP) servers.

This gateway is used with [GitHub Agentic Workflows](https://github.com/github/gh-aw) via the `sandbox.mcp` configuration to provide MCP server access to AI agents running in sandboxed environments.

📖 **[Full Configuration Specification](https://github.com/github/gh-aw/blob/main/docs/src/content/docs/reference/mcp-gateway.md)** - Complete reference for all configuration options and validation rules.

## Features

- **Configuration Modes**: Supports both TOML files and JSON stdin configuration
  - **Spec-Compliant Validation**: Fail-fast validation with detailed error messages
  - **Variable Expansion**: Environment variable substitution with `${VAR_NAME}` syntax
  - **Type Normalization**: Automatic conversion of legacy `"local"` type to `"stdio"`
- **Schema Normalization**: Automatic fixing of malformed JSON schemas from backend MCP servers
  - Adds missing `properties` field to object schemas
  - Prevents downstream validation errors
  - Transparent to both backends and clients
- **Routing Modes**:
  - **Routed**: Each backend server accessible at `/mcp/{serverID}`
  - **Unified**: Single endpoint `/mcp` that routes to configured servers
- **Docker Support**: Launch backend MCP servers as Docker containers
- **Stdio Transport**: JSON-RPC 2.0 over stdin/stdout for MCP communication
- **HTTP Transport**: Full support for HTTP-based MCP backends with session state preserved across requests
- **Container Detection**: Automatic detection of containerized environments with security warnings
- **Enhanced Debugging**: Detailed error context and troubleshooting suggestions for command failures
- **Per-ServerID Logs**: Separate log files for each backend MCP server (`{serverID}.log`) for easier troubleshooting

## Getting Started

For detailed setup instructions, building from source, and local development, see [CONTRIBUTING.md](CONTRIBUTING.md).

### Quick Start with Docker

1. **Pull the Docker image** (when available):
   ```bash
   docker pull ghcr.io/github/gh-aw-mcpg:latest
   ```

2. **Create a configuration file** (`config.json`):
   ```json
   {
     "gateway": {
       "apiKey": "${MCP_GATEWAY_API_KEY}"
     },
     "mcpServers": {
       "github": {
         "type": "stdio",
         "container": "ghcr.io/github/github-mcp-server:latest",
         "env": {
           "GITHUB_PERSONAL_ACCESS_TOKEN": ""
         }
       }
     }
   }
   ```

3. **Run the container**:
   ```bash
   docker run --rm -i \
     -e MCP_GATEWAY_PORT=8000 \
     -e MCP_GATEWAY_DOMAIN=localhost \
     -e MCP_GATEWAY_API_KEY=your-secret-key \
     -v /var/run/docker.sock:/var/run/docker.sock \
     -v /path/to/logs:/tmp/gh-aw/mcp-logs \
     -p 8000:8000 \
     ghcr.io/github/gh-aw-mcpg:latest < config.json
   ```

**Required flags:**
- `-i`: Enables stdin for passing JSON configuration
- `-e MCP_GATEWAY_*`: Required environment variables
- `-v /var/run/docker.sock`: Required for spawning backend MCP servers
- `-v /path/to/logs:/tmp/gh-aw/mcp-logs`: Mount for persistent gateway logs (or use `-e MCP_GATEWAY_LOG_DIR=/custom/path` with matching volume mount)
  - `mcp-gateway.log`: Unified log with all messages
  - `{serverID}.log`: Per-server logs for easier troubleshooting
  - `gateway.md`: Markdown-formatted logs for GitHub workflow previews
  - `rpc-messages.jsonl`: Machine-readable RPC message logs
  - `tools.json`: Available tools from all backend MCP servers
- `-p 8000:8000`: Port mapping must match `MCP_GATEWAY_PORT`

MCPG will start in routed mode on `http://0.0.0.0:8000` (using `MCP_GATEWAY_PORT`), proxying MCP requests to your configured backend servers.

## Configuration

MCP Gateway supports two configuration formats:
1. **TOML format** - Use with `--config` flag for file-based configuration
2. **JSON stdin format** - Use with `--config-stdin` flag for dynamic configuration

### TOML Format (`config.toml`)

TOML configuration requires `command = "docker"` for stdio-based MCP servers to ensure containerization:

```toml
[servers]

[servers.github]
command = "docker"
args = ["run", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN", "-i", "ghcr.io/github/github-mcp-server:latest"]
```

**Important**: Per [MCP Gateway Specification Section 3.2.1](https://github.com/github/gh-aw/blob/main/docs/src/content/docs/reference/mcp-gateway.md#321-containerization-requirement), all stdio-based MCP servers MUST be containerized. The gateway enforces this requirement by rejecting configurations where `command` is not `"docker"`.

**Why containerization is required:**
- Provides necessary process isolation and security boundaries
- Enables reproducible environments across different deployment contexts
- Container images provide versioning and dependency management
- Ensures portability and consistent behavior

For HTTP-based MCP servers, use the `url` field instead of `command`:

```toml
[servers.myhttp]
type = "http"
url = "https://example.com/mcp"
```

### JSON Stdin Format

For the complete JSON configuration specification with all validation rules, see the **[MCP Gateway Configuration Reference](https://github.com/github/gh-aw/blob/main/docs/src/content/docs/reference/mcp-gateway.md)**.

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "container": "ghcr.io/github/github-mcp-server:latest",
      "entrypoint": "/custom/entrypoint.sh",
      "entrypointArgs": ["--verbose"],
      "mounts": [
        "/host/config:/app/config:ro",
        "/host/data:/app/data:rw"
      ],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "",
        "EXPANDED_VAR": "${MY_HOME}/config"
      },
      "guard-policies": {
        "allow-only": {
          "repos": ["github/gh-aw-mcpg", "github/gh-aw"],
          "min-integrity": "unapproved"
        }
      }
    },
    "safeoutputs": {
      "type": "stdio",
      "container": "ghcr.io/github/safe-outputs:latest",
      "guard-policies": {
        "write-sink": {
          "accept": ["private:github/gh-aw-mcpg", "private:github/gh-aw"]
        }
      }
    }
  },
  "gateway": {
    "port": 8080,
    "apiKey": "your-api-key",
    "domain": "localhost",
    "startupTimeout": 30,
    "toolTimeout": 60,
    "payloadDir": "/tmp/jq-payloads"
  }
}
```

For complete field-by-field reference, see **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)**.

Key server fields: `type` (stdio/http), `container` (Docker image), `env` (environment variables with `${VAR}` expansion), `url` (HTTP endpoint), `tools` (tool filter list), `guard` (DIFC guard name), `guard-policies` (access control).

##### guard-policies

- **`allow-only`**: Restricts repository access for GitHub MCP servers — configures `repos` (scope) and `min-integrity` (none/unapproved/approved/merged)
- **`write-sink`**: Required for ALL output servers when DIFC guards are enabled — configures `accept` patterns matching agent secrecy tags

See **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)** for guard-policy details including the allow-only → write-sink accept mapping table.

#### Custom Schemas (`customSchemas`)

Define custom server types beyond `"stdio"` and `"http"` by mapping type names to HTTPS schema URLs. See **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)** for details.

#### Gateway Configuration Fields (Reserved)

| Field | Description | Default |
|-------|-------------|---------|
| `port` | HTTP port (1-65535) | From `--listen` flag |
| `apiKey` | API key for authentication | (disabled) |
| `domain` | Gateway domain | `localhost` |
| `startupTimeout` | Backend startup timeout (seconds) | `60` |
| `toolTimeout` | Tool execution timeout (seconds) | `120` |
| `payloadDir` | Large payload storage directory | `/tmp/jq-payloads` |

See **[docs/CONFIGURATION.md](docs/CONFIGURATION.md)** for TOML-only/CLI-only options and variable expansion features.

### Configuration Validation

The gateway provides fail-fast validation with precise error locations (line/column for TOML parse errors), unknown key detection (catches typos like `prot` instead of `port`), and environment variable expansion validation. Check log files for warnings after startup.

## Usage

Run `./awmg --help` for full CLI options. Key flags:

```bash
./awmg --config config.toml                    # TOML config file
./awmg --config-stdin < config.json            # JSON stdin
./awmg --config config.toml --routed           # Routed mode (default)
./awmg --config config.toml --unified          # Unified mode
./awmg --config config.toml --log-dir /path    # Custom log directory
```

## Environment Variables

For complete reference, see **[docs/ENVIRONMENT_VARIABLES.md](docs/ENVIRONMENT_VARIABLES.md)**.

Key variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `MCP_GATEWAY_PORT` | Gateway listening port | `8000` |
| `MCP_GATEWAY_API_KEY` | API key (reference in config via `"${MCP_GATEWAY_API_KEY}"`) | (disabled) |
| `MCP_GATEWAY_LOG_DIR` | Log file directory | `/tmp/gh-aw/mcp-logs` |
| `MCP_GATEWAY_PAYLOAD_DIR` | Payload storage directory | `/tmp/jq-payloads` |
| `MCP_GATEWAY_GUARDS_MODE` | DIFC mode: `strict`/`filter`/`propagate` | `strict` |
| `MCP_GATEWAY_WASM_GUARDS_DIR` | WASM guard directory | (disabled) |
| `DEBUG` | Debug logging pattern (e.g., `*`, `server:*`) | (disabled) |

## Containerized Mode

For production deployments, use `run_containerized.sh` which validates the environment, requires essential env vars, and checks Docker socket access:

```bash
docker run -i \
  -e MCP_GATEWAY_PORT=8080 \
  -e MCP_GATEWAY_DOMAIN=localhost \
  -e MCP_GATEWAY_API_KEY=your-key \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /path/to/logs:/tmp/gh-aw/mcp-logs \
  -p 8080:8080 \
  ghcr.io/github/gh-aw-mcpg:latest < config.json
```

Key flags: `-i` (required for stdin config), `-v .../docker.sock` (required for spawning backends), `-p` (must match `MCP_GATEWAY_PORT`).

For local development, use `run.sh` which provides defaults and warns about missing env vars.

## Logging

The gateway creates log files in the configured log directory (default: `/tmp/gh-aw/mcp-logs`):

| File | Purpose |
|------|---------|
| `mcp-gateway.log` | Unified log with all messages |
| `{serverID}.log` | Per-server logs (e.g., `github.log`) |
| `gateway.md` | Markdown-formatted logs for workflow previews |
| `rpc-messages.jsonl` | Machine-readable RPC messages |
| `tools.json` | Available tools from all backends |

Configure log location with `--log-dir` flag or `MCP_GATEWAY_LOG_DIR` env var. Logs include timestamps, levels (INFO/WARN/ERROR/DEBUG), categories, and contextual details.

For debug logging: `DEBUG=* ./awmg --config config.toml` (supports pattern matching: `DEBUG=server:*,launcher:*`)
## API Endpoints

### Routed Mode (default)

- `POST /mcp/{serverID}` - Send JSON-RPC request to specific server
  - Example: `POST /mcp/github` with body `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}`

### Unified Mode

- `POST /mcp` - Send JSON-RPC request (routed to first configured server)

### Health Check

- `GET /health` - Returns `OK`

## MCP Methods

Supported JSON-RPC 2.0 methods:

- `tools/list` - List available tools
- `tools/call` - Call a tool with parameters
- Any other MCP method (forwarded as-is)

## Security Features

### Authentication

Per MCP spec 7.1, the gateway uses plain API key authentication:
- Header format: `Authorization: <api-key>` (NOT Bearer scheme)
- Configure via `[gateway] api_key` in TOML, or `"gateway": {"apiKey": "${MCP_GATEWAY_API_KEY}"}` in JSON
- When configured, all endpoints except `/health` require authentication
- When not configured, authentication is disabled

### Enhanced Error Debugging

Command failures include full command details, environment variables, and context-specific troubleshooting suggestions (Docker connectivity, image availability, network issues, MCP compatibility).

## Architecture

Core MCP proxy with optional DIFC security:

- TOML and JSON stdin configuration with spec-compliant validation
- Environment variable expansion (`${VAR_NAME}`) with fail-fast behavior
- Stdio transport (containerized) and HTTP transport (session state preserved)
- Routed (`/mcp/{serverID}`) and unified (`/mcp`) modes
- Docker container launching for backend servers
- DIFC guard system with WASM-based guards

## MCP Server Compatibility

The gateway supports MCP servers via stdio transport using Docker containers. Tested with GitHub MCP and Serena MCP servers.

```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "container": "ghcr.io/github/github-mcp-server:latest"
    },
    "serena": {
      "type": "stdio",
      "container": "ghcr.io/github/serena-mcp-server:latest"
    }
  }
}
```

## Contributing

For development setup, build instructions, testing guidelines, and project architecture details, see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT License
