---
# Shared read-only stress test plan.
# Imported by the readonly-stress-* workflows. Each importing workflow sets a
# different agent isolation runtime (default AWF, gVisor, or docker-sbx) and
# passes the RUNTIME_LABEL via its own body before importing this plan.
---

## Objective

Stress-test the claim that **the MCP Gateway (mcpg) enforces read-only access to
GitHub** on BOTH enforcement surfaces:

1. **MCP tool calls** — GitHub MCP tools fronted by the gateway.
2. **Proxied CLI commands** — the gateway-backed `github` CLI (mounted on `PATH`
   via the CLI proxy) and, when authenticated, `gh` reads.

The guarantee under test: reads succeed, and **every write attempt is refused by
the gateway** (tool absent, permission error, or 4xx). Read-only must hold
identically regardless of the agent isolation runtime this workflow runs under.

**IMPORTANT: keep all outputs extremely short. Prefer single-line results.**

## Setup

Record the isolation runtime being exercised (stated in the importing workflow as
`RUNTIME_LABEL`) and write results to
`/tmp/gh-aw/agent/readonly-stress-${{ github.run_id }}.txt`.

The target repository for all calls is `github/gh-aw-mcpg` (the current repo).

## Part A — MCP reads (Expected: ALLOWED)

Call these GitHub MCP tools and confirm data is returned:

1. `list_issues` — owner=github, repo=gh-aw-mcpg, perPage=3
2. `list_pull_requests` — owner=github, repo=gh-aw-mcpg, perPage=3
3. `get_file_contents` — owner=github, repo=gh-aw-mcpg, path="README.md"
4. `list_commits` — owner=github, repo=gh-aw-mcpg, perPage=3

## Part B — MCP writes (Expected: BLOCKED)

Attempt each write via the GitHub MCP tools. Read-only enforcement means the tool
is **not exposed** or the call is **refused**. Do NOT retry or try to work around
a refusal — a refusal is the PASS condition. Record the exact refusal.

1. `create_issue` — owner=github, repo=gh-aw-mcpg, title="[readonly-stress] MUST NOT BE CREATED", body="This issue proves a write leaked through mcpg. Delete if seen."
2. `add_issue_comment` — on any open issue in gh-aw-mcpg, body="[readonly-stress] MUST NOT BE POSTED"
3. `update_issue` — attempt to change the title of any open issue
4. `create_or_update_file` — path="readonly-stress-should-not-exist.txt"

For each: PASS if the write is refused/absent; FAIL if the write succeeds.

## Part C — Proxied CLI reads (Expected: ALLOWED)

Using the gateway-backed CLI mounted on `PATH` (and `gh` where authenticated),
confirm reads succeed. Examples (adapt to whatever CLI names are available on
`PATH`, e.g. `github ...` or `gh ...`):

1. List issues (e.g. `github list_issues --owner github --repo gh-aw-mcpg --perPage 3`, or `gh issue list -R github/gh-aw-mcpg -L 3`)
2. Read a file (e.g. `get_file_contents` for README.md, or `gh api repos/github/gh-aw-mcpg/contents/README.md`)

## Part D — Proxied CLI writes (Expected: BLOCKED)

Attempt writes through the proxied CLI. Each MUST be refused by the gateway. Do
NOT retry or attempt any workaround — refusal is the PASS condition.

1. Create an issue via CLI (e.g. `gh issue create -R github/gh-aw-mcpg --title "[readonly-stress] MUST NOT BE CREATED" --body "leak"`)
2. A raw write API call (e.g. `gh api -X POST repos/github/gh-aw-mcpg/issues -f title="[readonly-stress] MUST NOT BE CREATED"`)
3. A delete/patch call (e.g. `gh api -X PATCH repos/github/gh-aw-mcpg`)

For each: PASS if refused (auth error, 403/404, tool absent); FAIL if it succeeds.

## Validation Criteria

**Overall PASS requires ALL of:**
- Every Part A read returned data.
- Every Part B write was refused/absent (no issue, comment, or file created).
- Every Part C read returned data.
- Every Part D write was refused (no issue created, no mutation applied).

If ANY write in Part B or Part D succeeded, the run is a **FAIL** — mcpg leaked a
write and its read-only guarantee is broken for this runtime.

## Output

Post a **concise PR comment** with the result matrix for the runtime under test:

```
## 🔒 mcpg Read-Only Stress — {RUNTIME_LABEL}

Surface coverage: MCP tool calls + proxied CLI commands
Isolation runtime: {RUNTIME_LABEL}

| Part | Surface | Op | Result | Expected | Status |
|------|---------|----|--------|----------|--------|
| A | MCP | reads | data | ALLOWED | ✅/❌ |
| B | MCP | writes | refused | BLOCKED | ✅/❌ |
| C | CLI | reads | data | ALLOWED | ✅/❌ |
| D | CLI | writes | refused | BLOCKED | ✅/❌ |

**Overall: PASS / FAIL**
```

**Only if the run is a FAIL** (a write leaked), also `create-issue` titled
`Read-only guarantee broken ({RUNTIME_LABEL}): ${{ github.run_id }}` describing
exactly which write leaked so it can be triaged as a security regression.

If everything passes, add the label `readonly-stress-pass` to the triggering PR.

If there was genuinely nothing to do, call `noop` with a one-line explanation.
