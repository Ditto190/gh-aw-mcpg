# Proxy Mode

Proxy mode (`awmg proxy`) is an HTTP forward proxy that intercepts GitHub API requests and applies DIFC (Data Information Flow Control) filtering using the same guard WASM module as the MCP gateway.

## Motivation

The MCP gateway enforces DIFC on MCP tool calls, but tools that call the GitHub API directly — such as `gh api`, `gh issue list`, or raw `curl` — bypass it entirely. Proxy mode closes this gap by sitting between the HTTP client and `api.github.com`, applying guard policies to REST and GraphQL requests.

## Quick Start

```bash
# Start the proxy
awmg proxy \
  --guard-wasm guards/github-guard/github_guard.wasm \
  --policy '{"allow-only":{"repos":["org/repo"],"min-integrity":"approved"}}' \
  --github-token "$GITHUB_TOKEN" \
  --listen localhost:8080

# Point gh CLI at the proxy
GH_HOST=localhost:8080 GH_TOKEN="$GITHUB_TOKEN" gh issue list -R org/repo

# Or use curl directly
curl -H "Authorization: token $GITHUB_TOKEN" \
  http://localhost:8080/api/v3/repos/org/repo/issues
```

## How It Works

```
HTTP client  →  awmg proxy (localhost:8080)  →  api.github.com
                       ↓
               6-phase DIFC pipeline
               (same guard WASM module)
```

1. The proxy receives an HTTP request (REST GET or GraphQL POST)
2. It maps the URL/query to a guard tool name (e.g., `/repos/:owner/:repo/issues` → `list_issues`)
3. The guard WASM module evaluates access based on the configured policy
4. If allowed, the request is forwarded to `api.github.com`
5. The response is filtered per-item based on secrecy/integrity labels
6. The filtered response is returned to the client

Write operations (PUT, POST, DELETE, PATCH) pass through unmodified.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--guard-wasm` | *(required)* | Path to the guard WASM module |
| `--policy` | | Guard policy JSON (e.g., `{"allow-only":{"repos":["org/repo"]}}`) |
| `--github-token` | `$GITHUB_TOKEN` | GitHub API token for upstream requests |
| `--listen` / `-l` | `127.0.0.1:8080` | HTTP listen address |
| `--log-dir` | `/tmp/gh-aw/mcp-logs` | Log file directory |
| `--guards-mode` | `filter` | DIFC mode: `strict`, `filter`, or `propagate` |
| `--github-api-url` | `https://api.github.com` | Upstream GitHub API URL |

## DIFC Pipeline

The proxy reuses the same 6-phase pipeline as the MCP gateway, with Phase 3 adapted for HTTP forwarding:

| Phase | Description | Shared with Gateway? |
|-------|-------------|---------------------|
| **0** | Extract agent labels from registry | ✅ |
| **1** | `Guard.LabelResource()` — coarse access check | ✅ |
| **2** | `Evaluator.Evaluate()` — secrecy/integrity evaluation | ✅ |
| **3** | Forward request to GitHub API | ❌ Proxy-specific |
| **4** | `Guard.LabelResponse()` — per-item labeling | ✅ |
| **5** | `Evaluator.FilterCollection()` — fine-grained filtering | ✅ |

## REST Route Mapping

The proxy maps ~25 GitHub REST API URL patterns to guard tool names:

| URL Pattern | Guard Tool |
|-------------|-----------|
| `/repos/:owner/:repo/issues` | `list_issues` |
| `/repos/:owner/:repo/issues/:number` | `get_issue` |
| `/repos/:owner/:repo/pulls` | `list_pull_requests` |
| `/repos/:owner/:repo/pulls/:number` | `get_pull_request` |
| `/repos/:owner/:repo/commits` | `list_commits` |
| `/repos/:owner/:repo/commits/:sha` | `get_commit` |
| `/repos/:owner/:repo/contents/:path` | `get_file_contents` |
| `/repos/:owner/:repo/branches` | `list_branches` |
| `/repos/:owner/:repo/releases` | `list_releases` |
| `/search/issues` | `search_issues` |
| `/search/code` | `search_code` |
| `/search/repositories` | `search_repositories` |
| `/user` | `get_me` |
| ... | See `internal/proxy/router.go` for full list |

Unrecognized URLs pass through without DIFC filtering.

## GraphQL Support

GraphQL queries to `/graphql` are parsed to extract the operation type and owner/repo context:

- **Repository-scoped queries** (issues, PRs, commits) — mapped to corresponding tool names
- **Search queries** — mapped to `search_issues` or `search_code`
- **Viewer queries** — mapped to `get_me`
- **Unknown queries** — passed through without filtering

Owner and repo are extracted from GraphQL variables (`$owner`, `$name`/`$repo`) or inline string arguments.

## Policy Notes

- **Repo names must be lowercase** in policies (e.g., `octocat/hello-world` not `octocat/Hello-World`). The guard performs case-insensitive matching against actual GitHub data.
- All policy formats supported by the MCP gateway work identically in proxy mode:
  - Specific repos: `{"allow-only":{"repos":["org/repo"]}}`
  - Owner wildcards: `{"allow-only":{"repos":["org/*"]}}`
  - Multiple repos: `{"allow-only":{"repos":["org/repo1","org/repo2"]}}`
  - Integrity filtering: `{"allow-only":{"repos":["org/repo"],"min-integrity":"approved"}}`

## Container Usage

The proxy is included in the same container image as the MCP gateway:

```bash
docker run --rm \
  --entrypoint /app/awmg \
  -p 8080:8080 \
  -e GITHUB_TOKEN \
  ghcr.io/github/gh-aw-mcpg:latest \
  proxy \
  --guard-wasm /guards/github/00-github-guard.wasm \
  --policy '{"allow-only":{"repos":["org/repo"],"min-integrity":"none"}}' \
  --github-token "$GITHUB_TOKEN" \
  --listen 0.0.0.0:8080 \
  --guards-mode filter
```

Note: The container entrypoint defaults to `run_containerized.sh` (MCP gateway mode). Use `--entrypoint /app/awmg` to run proxy mode directly.

## Guards Mode

| Mode | Behavior |
|------|----------|
| `strict` | Blocks entire response if any items are filtered |
| `filter` | Removes filtered items, returns remaining (default) |
| `propagate` | Labels accumulate on the agent; no filtering |

## Known Limitations

- **gh CLI HTTPS requirement**: `gh` forces HTTPS when connecting to `GH_HOST`. The proxy serves plain HTTP, so direct `gh` CLI interception requires a TLS-terminating reverse proxy in front. Use `curl` or `gh api --hostname` with HTTP for testing.
- **GraphQL nested filtering**: Deeply nested GraphQL response structures depend on guard support for item-level labeling.
- **Read-only filtering**: Only GET requests and GraphQL POST queries are filtered. Write operations pass through unmodified.
