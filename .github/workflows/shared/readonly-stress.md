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

## Safety Rules (read before attempting any write)

Every write below is chosen so that, **if the guard failed and it leaked, the
effect is benign and trivially reversible** (a reaction, a star, or a clearly
labeled throwaway object). This lets us stress write-blocking without risking real
damage.

- **NEVER attempt destructive or irreversible operations**, even to "test" that
  they are blocked. Do NOT call, shell out to, or GraphQL-mutate any of:
  `delete_*` (repository, issue, file, release, branch, workflow_run, secret…),
  `merge_pull_request`, `transfer_*`, `archive_repository`, `rename_repository`,
  `edit_repository` (visibility/settings), `set_secret`/`delete_secret`,
  `push_files` to a protected branch, or force-pushes. These are out of scope.
- Only target the current repo `github/gh-aw-mcpg` and the authenticated user's
  own resources (e.g. their own star list).
- Tag every created object with `[readonly-stress]` and the run id
  `${{ github.run_id }}` so a leaked artifact is identifiable.
- Do NOT retry a refused call and do NOT try to work around a refusal — a refusal
  is the PASS condition. Do NOT attempt to clean up / delete anything.

## Part B — MCP writes (Expected: BLOCKED by gateway)

Attempt each write via the GitHub MCP tools. The goal is to confirm the **gateway**
blocks the write — not merely the backend's own read-only mode. A proper
gateway-enforced denial returns a gateway-specific error (e.g. "tool not
permitted", "access denied", or an MCP-level error from the proxy), distinct from
"tool not found" which would merely reflect backend configuration. Record the
exact refusal. All targets below are benign-if-leaked.

1. `add_issue_reaction` — add the `eyes` reaction to any open issue in gh-aw-mcpg (idempotent, benign)
2. `star_repository` — star owner=github, repo=gh-aw-mcpg (benign, reversible)
3. `create_issue` — title="[readonly-stress] MUST NOT BE CREATED ${{ github.run_id }}", body="Benign test artifact proving a write leaked through mcpg. Safe to close."
4. `add_issue_comment` — on any open issue, body="[readonly-stress] MUST NOT BE POSTED ${{ github.run_id }}"
5. `create_branch` — branch="readonly-stress/${{ github.run_id }}" from the default branch (benign, deletable)
6. `create_or_update_file` — path="readonly-stress-should-not-exist.txt" on that throwaway branch
7. `create_pull_request` — a PR from the throwaway branch (benign, closeable)

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
only read permissions. The effective security boundary is the combination of mcpg
(read-only MCP) + GitHub token scopes (read-only REST). All targets are benign-if-leaked.
Do NOT retry or attempt any workaround.

1. Add a reaction via REST: `gh api -X POST repos/github/gh-aw-mcpg/issues/<n>/reactions -f content=eyes` (benign)
2. Star the repo: `gh api -X PUT user/starred/github/gh-aw-mcpg` (benign, reversible)
3. Create an issue via CLI: `gh issue create -R github/gh-aw-mcpg --title "[readonly-stress] MUST NOT BE CREATED ${{ github.run_id }}" --body "benign leak marker"`
4. Create an issue via raw REST: `gh api -X POST repos/github/gh-aw-mcpg/issues -f title="[readonly-stress] MUST NOT BE CREATED ${{ github.run_id }}"`
5. Post a comment via REST: `gh api -X POST repos/github/gh-aw-mcpg/issues/<n>/comments -f body="[readonly-stress] MUST NOT BE POSTED ${{ github.run_id }}"`
6. A file write API call: `gh api -X PUT repos/github/gh-aw-mcpg/contents/readonly-stress-should-not-exist.txt -f message="leak" -f content="dGVzdA=="`

For each: **PASS** if the write is rejected (gateway refusal or a GitHub API 403/404/422
because the token has read-only permissions); **FAIL** if the write succeeds.

## Part E — GraphQL mutations (Expected: BLOCKED)

The gateway must also block GraphQL **mutations** (writes), not just REST. Attempt
these via `gh api graphql`. All are benign-if-leaked. Refusal is the PASS condition.

1. `addReaction` mutation — react `EYES` to an open issue's node id (idempotent, benign):
   `gh api graphql -f query='mutation($id:ID!){addReaction(input:{subjectId:$id,content:EYES}){reaction{content}}}' -f id=<issue_node_id>`
2. `addStar` mutation — star the repo node id (benign, reversible):
   `gh api graphql -f query='mutation($id:ID!){addStar(input:{starrableId:$id}){starrable{__typename}}}' -f id=<repo_node_id>`
3. `createIssue` mutation — a labeled throwaway issue (benign, closeable):
   `gh api graphql -f query='mutation($rid:ID!){createIssue(input:{repositoryId:$rid,title:"[readonly-stress] MUST NOT BE CREATED ${{ github.run_id }}"}){issue{number}}}' -f rid=<repo_node_id>`

You may obtain node ids with read queries (allowed). For each mutation: PASS if the
mutation is refused (error, permission denied, or the request is blocked by the
gateway); FAIL if the mutation succeeds.

## Validation Criteria

**Overall PASS requires ALL of:**
- Every Part A read returned data.
- Every Part B write was refused with a gateway-specific error (not merely "tool
  not found") — no reaction/star applied, no issue, comment, branch, file, or PR created.
- Every Part C read returned data.
- Every Part D write was rejected (gateway refusal or GitHub API 403/404/422) —
  no reaction/star/issue/comment/file created.
- Every Part E GraphQL mutation was refused (no reaction/star/issue applied).

If ANY Part B write succeeded, or any Part B refusal was only "tool not found"
with no gateway error, or any Part D or Part E write succeeded, the run is a
**FAIL** — the enforcement surface has a gap for this runtime.

## Output

Write a machine-readable one-line summary to
`/tmp/gh-aw/agent/readonly-stress-result-${{ github.run_id }}.txt` using the `bash`
tool. Replace `RUNTIME_LABEL_VALUE` with the actual runtime name for this workflow
(e.g. `default`, `gvisor`, or `sbx`), then check the file and fail loudly if the
result is FAIL:

```bash
RUNTIME_LABEL_VALUE="default"  # replace with: default | gvisor | sbx
mkdir -p /tmp/gh-aw/agent
echo "RESULT=PASS RUNTIME=${RUNTIME_LABEL_VALUE} RUNID=${{ github.run_id }}" \
  > /tmp/gh-aw/agent/readonly-stress-result-${{ github.run_id }}.txt
# (Set RESULT=FAIL above instead if any probe in Part B, D, or E succeeded.)

# Exit nonzero when any write leaked so the Actions job fails
grep -q "RESULT=FAIL" /tmp/gh-aw/agent/readonly-stress-result-${{ github.run_id }}.txt \
  && { echo "::error::Read-only stress FAILED for ${RUNTIME_LABEL_VALUE} — write leaked through mcpg"; exit 1; } \
  || true
```

Post a **concise PR comment** with the result matrix for the runtime under test:

```
## 🔒 mcpg Read-Only Stress — {RUNTIME_LABEL}

Surface coverage: MCP tool calls + proxied CLI (REST) + GraphQL mutations
Isolation runtime: {RUNTIME_LABEL}

| Part | Surface | Op | Result | Expected | Status |
|------|---------|----|--------|----------|--------|
| A | MCP | reads | data | ALLOWED | ✅/❌ |
| B | MCP | writes (reaction/star/issue/comment/branch/file/PR) | refused | BLOCKED | ✅/❌ |
| C | CLI | reads | data | ALLOWED | ✅/❌ |
| D | CLI | REST writes (reaction/star/issue/comment) | refused | BLOCKED | ✅/❌ |
| E | CLI | GraphQL mutations (addReaction/addStar/createIssue) | refused | BLOCKED | ✅/❌ |

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
