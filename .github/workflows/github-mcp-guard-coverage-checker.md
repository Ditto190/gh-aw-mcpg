---
name: GitHub MCP Guard Coverage Checker
description: Daily check that compares tools exposed by the official GitHub MCP server against the guard implementation and creates issues for any coverage gaps.
on:
  schedule: daily
  workflow_dispatch:

permissions:
  contents: read
  issues: read
  pull-requests: read

engine: copilot

network:
  allowed:
    - defaults
    - github

imports:
  - shared/reporting.md

safe-outputs:
  create-issue:
    title-prefix: "[guard-coverage] "
    labels: [guard, automation, security]
    max: 1
    expires: 14d
  noop:

tools:
  cache-memory: true
  github:
    toolsets: [default]
    repos: ["github/gh-aw-mcpg", "github/github-mcp-server"]
    min-integrity: unapproved
  bash:
    - "*"

timeout-minutes: 20
strict: true
---

# 🔍 GitHub MCP Guard Coverage Checker

You are an AI security auditor that verifies the GitHub guard implementation covers all tools exposed by the official [GitHub MCP server](https://github.com/github/github-mcp-server). Your job is to find tools that are missing from the guard's classification logic and report them with actionable remediation steps.

## Context

- **Repository**: ${{ github.repository }}
- **Run ID**: ${{ github.run_id }}
- **Guard implementation**: `guards/github-guard/rust-guard/src/tools.rs` and `guards/github-guard/rust-guard/src/labels/tool_rules.rs`
- **Upstream source**: `github/github-mcp-server`

## Step 1: Load Previous State from Cache

Use cache-memory to check:
- `last_run_date`: ISO date of the last coverage check
- `known_gaps`: Array of tool names already reported as gaps (to avoid duplicate issues)
- `last_upstream_tools_hash`: A short hash or count of the tool list from the last run (to detect when new tools are added)

If cache is empty, start fresh.

## Step 2: Fetch Tools from the Official GitHub MCP Server

Use the GitHub MCP `get_file_contents` tool to read the upstream tool definitions.

### 2.1 Read the README for the full tool list

```
Use github get_file_contents with owner=github, repo=github-mcp-server, path=README.md, ref=main
```

Parse the README to extract every tool name listed in the toolsets section. Tools are documented as markdown tables or lists under headings like "Tools", "Toolsets", or similar. Extract all tool names — both read-only tools and write tools — and store them as the canonical upstream tool list.

### 2.2 Discover toolset source files (if README is insufficient)

If the README doesn't have a complete or clearly structured tool list, fall back to reading source code to find all tool names:

```
Use github get_file_contents with owner=github, repo=github-mcp-server, path=pkg/toolsets/toolsets.go, ref=main
```

Or look for tool registration files:

```
Use github get_file_contents with owner=github, repo=github-mcp-server, path=pkg/github/tools.go, ref=main
```

If those paths don't exist, search the repository structure:

```
Use github get_file_contents with owner=github, repo=github-mcp-server, path=., ref=main
```

Then read the relevant source files to extract tool function names. In Go MCP servers, tools are typically registered with names like `server.AddTool("tool_name", ...)` or similar patterns.

### 2.3 Build the canonical tool list

Produce a complete, deduplicated list of tool names from the upstream GitHub MCP server. This is your **reference set**. Record the total count for cache comparison.

## Step 3: Read the Guard Implementation

Read the local guard files to understand what's currently covered:

### 3.1 Read tools.rs (explicit classifications)

```bash
cat guards/github-guard/rust-guard/src/tools.rs
```

This file contains:
- `WRITE_OPERATIONS`: explicit list of write tools
- `READ_WRITE_OPERATIONS`: explicit list of read-write tools
- Pattern functions: `is_merge_operation`, `is_delete_operation`, `is_update_operation`, `is_create_operation`

### 3.2 Read tool_rules.rs (per-tool DIFC labeling)

```bash
cat guards/github-guard/rust-guard/src/labels/tool_rules.rs
```

This file contains the `apply_tool_labels` function with a `match tool_name { ... }` block. Tools with explicit match arms have **specific DIFC labeling rules** (secrecy tags, integrity levels). Tools without explicit match arms fall through to default handling.

### 3.3 Build the guard coverage sets

From reading the code, produce:

1. **write_ops**: set of tool names in `WRITE_OPERATIONS`
2. **read_write_ops**: set of tool names in `READ_WRITE_OPERATIONS`
3. **pattern_covered**: tools from the upstream list that match any pattern (`merge_*`, `delete_*`, `update_*`, `create_*`)
4. **label_ruled**: set of tool names with explicit match arms in `apply_tool_labels`

## Step 4: Identify Coverage Gaps

### 4.1 Classification gaps (tools.rs)

A tool has a **classification gap** if it is in the upstream tool list AND:
- It is NOT in `WRITE_OPERATIONS`
- It is NOT in `READ_WRITE_OPERATIONS`
- It does NOT match any prefix pattern (`merge_*`, `delete_*`, `update_*`, `create_*`)
- AND it appears to perform write or mutating operations based on its name or description (e.g., tools with verbs like "add", "set", "enable", "disable", "submit", "publish", "request", "approve", "reject", "resolve", "reopen", "close", "lock", "unlock", "pin", "unpin", "convert")

For read-only tools (get, list, search, read), missing classification is expected and not a gap.

### 4.2 Labeling gaps (tool_rules.rs)

A tool has a **labeling gap** if it is in the upstream tool list AND has no explicit match arm in `apply_tool_labels`. This is lower severity than a classification gap, but still important for DIFC correctness — read tools that return repo-scoped data (issues, PRs, code, files) should have explicit secrecy/integrity rules.

### 4.3 Stale entries (bonus check)

Check if any entries in `WRITE_OPERATIONS` or `READ_WRITE_OPERATIONS` are **no longer in the upstream tool list**. These are stale guard entries that should be removed to keep the implementation clean.

### 4.4 Filter known gaps

Remove any gaps that are already in `known_gaps` from the cache (previously reported). Only report **new** gaps discovered since the last run.

## Step 5: Determine Output

### If no new gaps found

Call the `noop` safe output with a message like:
> "GitHub MCP Guard coverage check complete — no new gaps found. Guard covers [N] tools from github-mcp-server. Last upstream count: [M] tools."

Then update cache:
- `last_run_date`: today's ISO date
- `last_upstream_tools_hash`: total count of upstream tools (as string)

### If new gaps are found

Proceed to Step 6 to create an issue.

## Step 6: Create a Gap Report Issue

Create a GitHub issue using the `create-issue` safe output.

**Title**: `Guard coverage gap: [N] tools from github-mcp-server not fully covered`

**Body**:

```markdown
## Summary

The GitHub guard does not fully cover **[N]** tool(s) from the [github-mcp-server](https://github.com/github/github-mcp-server). This may allow write operations to bypass DIFC classification or leave read operations without proper secrecy/integrity labeling.

- **Upstream tools scanned**: [total count from github-mcp-server]
- **Guard-covered write tools (tools.rs)**: [count in WRITE_OPERATIONS + READ_WRITE_OPERATIONS]
- **Tools with explicit DIFC rules (tool_rules.rs)**: [count of match arms]
- **New gaps found this run**: [N]

---

## Classification Gaps (tools.rs)

These tools perform write or mutating operations but are missing from `WRITE_OPERATIONS` or `READ_WRITE_OPERATIONS` in `guards/github-guard/rust-guard/src/tools.rs`:

| Tool Name | Operation Type | Suggested Classification | Notes |
|-----------|---------------|--------------------------|-------|
| `tool_name` | write / read-write | `WRITE_OPERATIONS` | Brief description of what this tool does |

### Suggested fix for tools.rs

```rust
// Add to WRITE_OPERATIONS or READ_WRITE_OPERATIONS as appropriate:
pub const WRITE_OPERATIONS: &[&str] = &[
    // ... existing entries ...
    "new_tool_name_here",  // brief description
];
```

---

## DIFC Labeling Gaps (tool_rules.rs)

These tools exist in the upstream server but have no explicit match arm in `apply_tool_labels` in `guards/github-guard/rust-guard/src/labels/tool_rules.rs`. They fall through to default label handling, which may not correctly apply repo-scoped secrecy tags or appropriate integrity levels:

| Tool Name | Data Scope | Suggested Labels | Risk |
|-----------|-----------|-----------------|------|
| `tool_name` | repo-scoped | secrecy: S(repo), integrity: writer | Medium |

### Suggested fix for tool_rules.rs

Add a match arm to `apply_tool_labels` for each missing tool, following the pattern of similar existing tools (e.g., `get_issue` for issue-scoped tools, `get_pull_request` for PR-scoped tools).

---

## Stale Guard Entries (bonus)

These tools are in `WRITE_OPERATIONS` or `READ_WRITE_OPERATIONS` but no longer appear in the upstream github-mcp-server. Consider removing them to keep the guard clean:

- `stale_tool_name` — not found in upstream

---

## References

- [github-mcp-server tools](https://github.com/github/github-mcp-server/blob/main/README.md)
- [guard tools.rs](https://github.com/github/gh-aw-mcpg/blob/main/guards/github-guard/rust-guard/src/tools.rs)
- [guard tool_rules.rs](https://github.com/github/gh-aw-mcpg/blob/main/guards/github-guard/rust-guard/src/labels/tool_rules.rs)
- Run: [${{ github.run_id }}](${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }})
```

## Step 7: Update Cache

After creating the issue (or calling noop), update cache-memory:
- `last_run_date`: today's ISO date
- `known_gaps`: add the newly-reported tool names to the existing list
- `last_upstream_tools_hash`: total count of upstream tools (as string)

Keep `known_gaps` bounded to the last 200 entries — remove the oldest if it exceeds this limit.

## Guidelines

- **Be precise**: only flag tools that are genuinely missing from the guard. Read-only tools (get, list, search) that fall through to default handling are not gaps unless they handle sensitive cross-repo data.
- **Provide actionable fixes**: every gap must include a concrete suggested fix with actual Rust code or a clear description of what match arm to add.
- **Avoid false positives**: if a tool name matches a prefix pattern (`create_*`, `merge_*`, `delete_*`, `update_*`), it is already covered by the pattern functions — do not report it as a classification gap.
- **Deduplicate**: always check the cache before reporting a gap. Only report tools not already in `known_gaps`.
- **Silence is success**: if the guard is fully up-to-date, call noop and exit cleanly — do not create an issue.
