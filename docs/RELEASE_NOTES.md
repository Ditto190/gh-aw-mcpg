# Release Notes

## v0.3.18

This release focuses on **hardening the WASM guard subsystem**, improving code quality through targeted refactoring, and expanding test coverage for the Rust guard and collaborator permission packages.

### ✨ What's New

- **WASM guard robustness** ([#6290](https://github.com/github/gh-aw-mcpg/pull/6290), [#6296](https://github.com/github/gh-aw-mcpg/pull/6296)): The wazero-based guard runtime now handles oversized `call_backend` responses via a size-hint protocol, uses larger I/O buffers, improves cache reconfiguration locking, and adds fallback-path coverage — making guard execution more reliable under high-load and edge-case conditions.
- **DIFC flags module** ([#6243](https://github.com/github/gh-aw-mcpg/pull/6243)): Guard policy override logic has been refactored into a dedicated DIFC flags module, improving maintainability and consistency of security policy enforcement.

### 🐛 Bug Fixes & Improvements

- **Config map expansion** ([#6289](https://github.com/github/gh-aw-mcpg/pull/6289)): Stdin config map expansion no longer duplicates environment/header logic, reducing the risk of subtle configuration drift.
- **Flag/env override helper** ([#6288](https://github.com/github/gh-aw-mcpg/pull/6288)): A shared `applyFlagOrEnv` helper eliminates duplicated flag-override patterns across CLI commands.

### 🔬 Testing & Reliability

- Expanded Rust guard test coverage for GraphQL node paths and GitHub URL repo extraction ([#6284](https://github.com/github/gh-aw-mcpg/pull/6284), [#6291](https://github.com/github/gh-aw-mcpg/pull/6291)).
- Improved unit tests for the MCP collaborator permission package ([#6249](https://github.com/github/gh-aw-mcpg/pull/6249)).
- Added debug logging to `proxy/graphql_rewrite.go` for easier diagnostics ([#6248](https://github.com/github/gh-aw-mcpg/pull/6248)).

### 🐳 Docker Image

The Docker image for this release is available at:

```bash
docker pull ghcr.io/github/gh-aw-mcpg:v0.3.18
# or
docker pull ghcr.io/github/gh-aw-mcpg:latest
```

Supported platforms: `linux/amd64`, `linux/arm64`

---

For complete details, see the [full release notes](https://github.com/github/gh-aw-mcpg/releases/tag/v0.3.18).
