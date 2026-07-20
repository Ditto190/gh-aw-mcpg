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

## Part B — MCP writes (Expected: BLOCKED by gateway)

Attempt each write via the GitHub MCP tools. The goal is to confirm the **gateway**
blocks the write — not the backend's own read-only mode. A proper gateway-enforced
denial returns a gateway-specific error (e.g. "tool not permitted", "access
denied", or an MCP-level error from the proxy), and is distinct from "tool not
found" which would merely reflect backend configuration.

Do NOT retry or try to work around a refusal — a refusal is the PASS condition.
Record the exact error message or refusal reason returned.

1. `issue_write` — owner=github, repo=gh-aw-mcpg, operation="create", title="[readonly-stress] MUST NOT BE CREATED", body="This issue proves a write leaked through mcpg. Delete if seen."
   - (Fallback if `issue_write` is unavailable: try `create_issue` with the same args.)
2. `issue_write` — owner=github, repo=gh-aw-mcpg, operation="comment", issue_number=1, body="[readonly-stress] MUST NOT BE POSTED"
   - (Fallback: `add_issue_comment` on any open issue.)
3. `issue_write` — owner=github, repo=gh-aw-mcpg, operation="update", issue_number=1, title="[readonly-stress] MUST NOT BE UPDATED"
   - (Fallback: `update_issue` with the same args.)
4. `create_or_update_file` — owner=github, repo=gh-aw-mcpg, path="readonly-stress-should-not-exist.txt", content="dGVzdA==", message="[readonly-stress] MUST NOT BE COMMITTED", branch="main"
   (All required fields populated so the call is a valid write attempt.)

For each: **PASS** if the gateway refuses with a gateway-specific error; **FAIL**
if the write succeeds or if the only refusal is "tool not found" (which tests
backend config, not gateway enforcement).

## Part C — Proxied CLI reads (Expected: ALLOWED)

Using the gateway-backed CLI mounted on `PATH` (and `gh` where authenticated),
confirm reads succeed. Examples (adapt to whatever CLI names are available on
`PATH`, e.g. `github ...` or `gh ...`):

1. List issues (e.g. `github list_issues --owner github --repo gh-aw-mcpg --perPage 3`, or `gh issue list -R github/gh-aw-mcpg -L 3`)
2. Read a file (e.g. `get_file_contents` for README.md, or `gh api repos/github/gh-aw-mcpg/contents/README.md`)

## Part D — Proxied CLI writes (Expected: BLOCKED by GitHub API permissions)

Attempt writes through the proxied CLI. Note: the mcpg proxy passes non-GET/non-GraphQL
REST requests through to GitHub unchanged, so write CLI commands are not blocked by the
gateway itself — they are rejected by the GitHub API because this job's token carries
only `contents: read` and `issues: read` permissions. The effective security boundary is
the combination of mcpg (read-only MCP) + GitHub token scopes (read-only REST).

Do NOT retry or attempt any workaround.

1. Create an issue via CLI: `gh issue create -R github/gh-aw-mcpg --title "[readonly-stress] MUST NOT BE CREATED" --body "leak"`
2. A raw write API call: `gh api -X POST repos/github/gh-aw-mcpg/issues -f title="[readonly-stress] MUST NOT BE CREATED" -f body="leak"`
3. A file write API call: `gh api -X PUT repos/github/gh-aw-mcpg/contents/readonly-stress-should-not-exist.txt -f message="leak" -f content="dGVzdA=="`

For each: **PASS** if the GitHub API returns a 403/404/422 (write rejected because the
token has read-only permissions); **FAIL** if the write succeeds (issue or file created).

## Validation Criteria

**Overall PASS requires ALL of:**
- Every Part A read returned data.
- Every Part B write was refused with a gateway-specific error (not merely "tool not found").
- Every Part C read returned data.
- Every Part D write was rejected by the GitHub API with 403/404/422 (no issue or file created).

If ANY Part B write succeeded, or any Part B refusal was only "tool not found" with no
gateway error, or any Part D write succeeded, the run is a **FAIL** — the enforcement
surface has a gap for this runtime.

## Output

Write a machine-readable one-line summary to
`/tmp/gh-aw/agent/readonly-stress-result-${{ github.run_id }}.txt` using the `bash`
tool, then check it and fail loudly if the result is FAIL:

```bash
mkdir -p /tmp/gh-aw/agent
echo "RESULT=<PASS|FAIL> RUNTIME=<RUNTIME_LABEL> RUNID=${{ github.run_id }}" \
  > /tmp/gh-aw/agent/readonly-stress-result-${{ github.run_id }}.txt

# Exit nonzero when any write leaked so the Actions job fails
grep -q "RESULT=FAIL" /tmp/gh-aw/agent/readonly-stress-result-${{ github.run_id }}.txt \
  && { echo "::error::Read-only stress FAILED for ${RUNTIME_LABEL} — write leaked through mcpg"; exit 1; } \
  || true
```

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

If everything passes, add the runtime-specific label
`readonly-stress-pass-{RUNTIME_LABEL}` to the triggering PR (e.g.
`readonly-stress-pass-default`, `readonly-stress-pass-gvisor`,
`readonly-stress-pass-sbx`).

If there was genuinely nothing to do, call `noop` with a one-line explanation.
