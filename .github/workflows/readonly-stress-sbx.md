---
emoji: 🔒
description: PR stress test proving mcpg enforces read-only GitHub access (MCP tool calls + proxied CLI) under the docker-sbx microVM runtime
on:
  pull_request:
    types: [opened, synchronize, reopened]
  workflow_dispatch:
  reaction: "eyes"
permissions:
  contents: read
  issues: read
  pull-requests: read
  actions: read
  copilot-requests: write
name: "Read-Only Stress: docker-sbx runtime"
engine:
  id: copilot
strict: false
inlined-imports: true
imports:
  - shared/reporting.md
  - shared/readonly-stress.md
network:
  allowed:
    - defaults
    - github
    - github.com
tools:
  github:
    mode: local
    toolsets: [repos, issues, pull_requests, search]
    min-integrity: approved
  cli-proxy: true
  edit:
  bash:
    - "github"
    - "gh"
    - "cat"
    - "echo"
    - "date"
    - "jq"
    - "mkdir"
    - "grep"
    - "wc"
    - "head"
    - "tail"
sandbox:
  agent:
    id: awf
    runtime: docker-sbx
    sudo: true
  mcp:
    container: "ghcr.io/github/gh-aw-mcpg"
    version: "latest"
safe-outputs:
  threat-detection:
    enabled: false
  add-comment:
    hide-older-comments: true
    max: 2
  create-issue:
    max: 1
  add-labels:
    allowed: [readonly-stress-pass]
  messages:
    footer: "> 🔒 *mcpg read-only stress (docker-sbx runtime) by [{workflow_name}]({run_url})*"
    run-started: "🔒 [{workflow_name}]({run_url}) is stress-testing mcpg read-only enforcement under the docker-sbx microVM runtime..."
    run-success: "🔒 [{workflow_name}]({run_url}) completed. Read-only enforcement validated. ✅"
    run-failure: "🔒 [{workflow_name}]({run_url}) reports {status}. Read-only enforcement may be broken. ⚠️"
timeout-minutes: 15
---

# mcpg Read-Only Stress Test — docker-sbx Runtime

`RUNTIME_LABEL` = **docker-sbx (KVM-isolated microVM)**.

This run exercises the gateway's read-only guarantee while the agent runs inside a
**docker-sbx** microVM (`sandbox.agent.runtime: docker-sbx`), which provides
hardware-virtualization (KVM) isolation while infrastructure containers (the mcpg
gateway and MCP servers) remain on the host. Follow the shared test plan below,
attempting reads (expect ALLOWED) and writes (expect BLOCKED) on both the MCP
tool-call surface and the proxied CLI surface. Read-only must hold identically to
the default runtime.

## Usage

The docker-sbx runtime requires a KVM-capable runner and the `DOCKER_PAT` /
`DOCKER_USERNAME` repository secrets (Docker Hub OAuth) so the microVM image can
be pulled. If those prerequisites are unavailable, run the `default` and `gvisor`
variants instead.
