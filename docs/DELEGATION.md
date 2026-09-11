# Delegation Control

Delegation control (`github-repository-delegation-v1`) lets the Agentic Workflow Firewall (AWF)
mint short-lived, invocation-scoped identities that grant a dynamically admitted agent enclave
read-only access to exactly one GitHub repository, without adding, removing, or mutating any
configured MCP backend, route, or tool.

The feature is **disabled by default**. It is activated only when all five
`MCP_GATEWAY_DELEGATION_*` environment variables are set, and it is intended to be configured by
the workflow compiler — not by hand.

It is implemented in `internal/delegation/` and wired into both modes:

| Mode | Data plane | Control plane |
|------|------------|---------------|
| `awmg proxy` | Delegated GitHub REST reads on the proxy listener (`internal/proxy/delegation.go`) | Private control listener |
| Unified gateway (`awmg --unified`) | Delegated MCP tool calls on `/mcp` (`internal/server/delegation.go`) | Private control listener |

## Environment Variables

All five variables must be set together. If any subset (but not all) is set, startup fails fast
with an error naming the variables.

| Variable | Description |
|----------|-------------|
| `MCP_GATEWAY_DELEGATION_ENVELOPE` | JSON delegation envelope installed by the workflow compiler. Decoded with unknown fields rejected; trailing JSON is an error. See [Envelope format](#envelope-format). |
| `MCP_GATEWAY_DELEGATION_STATE_PATH` | Filesystem path where delegated identity state is persisted (atomically) and from which it is recovered on restart. |
| `MCP_GATEWAY_DELEGATION_GENERATION` | Unsigned integer policy generation. Persisted state whose generation does not match is rejected and the store fails closed. |
| `MCP_GATEWAY_DELEGATION_CONTROL_KEY` | AWF-only capability secret authenticating control-plane requests. Must be at least 32 bytes. Must never be given to the primary agent or to an enclave executor. |
| `MCP_GATEWAY_DELEGATION_CONTROL_LISTEN` | Address (`host:port`) of the private control listener. It must be distinct from the executor-facing proxy/gateway listener and must not be reachable by agents. |

## Envelope format

The envelope is the compiler-installed, compiler-bounded policy for the whole run. Every delegated
identity must be a strict subset of it, and the envelope is immutable for the lifetime of the
process. Duration fields are whole seconds on the wire.

```json
{
  "run_id": "18234567890",
  "enclave_backend": "awf-enclave",
  "allowed_repositories": ["octo-org/octo-repo"],
  "allowed_owners": ["octo-org"],
  "tool_policy": "github-repository-read-v1",
  "allowed_schema_hashes": ["b1946ac92492d2347c6235b4d2611184"],
  "max_dynamic_schema_hashes": 8,
  "max_identity_ttl": 900,
  "expires_at": "2026-09-11T00:00:00Z"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `run_id` | yes | Workflow run every identity minted from this envelope is bound to. |
| `enclave_backend` | yes | The single AWF enclave backend identities may bind to. |
| `allowed_repositories` | yes, unless `allowed_owners` is set | Closed set of canonical `owner/repo` selectors, compared as exact ASCII bytes (no normalization). Duplicates are rejected. |
| `allowed_owners` | yes, unless `allowed_repositories` is set | Closed set of canonical owners; AWF may select any exact repository under an allowed owner at invocation time. One identity is still bound to exactly one repository. |
| `tool_policy` | yes | Must be `github-repository-read-v1`, a closed allowlist of the repository-scoped read-only tools `issue_read` and `list_issues`. |
| `allowed_schema_hashes` | yes, unless `max_dynamic_schema_hashes` > 0 | Closed set of approved finite response schema hashes. |
| `max_dynamic_schema_hashes` | yes, unless `allowed_schema_hashes` is non-empty | Bounds how many distinct invocation-supplied schema hashes may be admitted at runtime. Has no effect when `allowed_schema_hashes` is non-empty. |
| `max_identity_ttl` | yes | Whole seconds; upper bound on how long any single identity (and therefore any executor bearer) may live. |
| `expires_at` | yes | RFC 3339 absolute envelope expiry. No identity may be created after it. |

## Control channel

When delegation is enabled, a private HTTP control channel listens on
`MCP_GATEWAY_DELEGATION_CONTROL_LISTEN`, separate from the executor-facing data plane, so that
executor traffic cannot reach control operations even with a valid executor bearer.

Every request must be a `POST` carrying the capability in the `Authorization` header, either bare
(per MCP spec 7.1) or with a leading `Bearer` scheme prefix. Anything else returns
`403 delegation_access_denied`.
Request bodies are limited to 64 KiB and decoded with unknown fields rejected.

Endpoints are served under `/internal/awf-enclave-mcp-control/`:

| Path | Request body | Response |
|------|--------------|----------|
| `create-or-confirm` | `run_id`, `enclave_backend`, `enclave_entry_id`, `invocation_id`, `repository`, `tool_policy`, `schema_hash`, optional `admitted_default_branch_sha`, `requested_ttl` (seconds), optional `invocation_expires_at`, `idempotency_key` | `handle`, `executor_bearer`, `repository`, `tool_policy`, `tools`, optional `admitted_default_branch_sha`, `expires_at` |
| `revoke` | `handle` | `{"revoked": true}` |
| `revoke-by-labels` | `run_id`, `enclave_entry_id` | `{"revoked": <count>}` |
| `status` | `run_id`, `enclave_entry_id` | `recovery_incomplete`, `generation`, `live_identity_count`, `labelled_handles` |
| `reconcile` | `{}` | `{"reconciled": true}` |

`create-or-confirm` is idempotent per `(run_id, enclave_entry_id, invocation_id)` tuple: a repeated
call returns the same binding, expiry, handle, and executor bearer. A request whose binding differs
from the stored identity is terminal — the stored identity is revoked and the request is denied.

Requests outside the envelope (unknown repository or owner, wrong tool policy, unapproved schema
hash, TTL above `max_identity_ttl`, elapsed invocation deadline) are denied with
`403 delegation_request_denied`.

## Restart recovery

State is persisted to `MCP_GATEWAY_DELEGATION_STATE_PATH` after every mutating control operation
and reloaded at startup. Recovery is all-or-nothing for an existing state file: if it is unreadable, fails
integrity verification, or does not match the active generation, the store fails closed with no live
identities. A missing file is treated as a fresh start. Restored identities that are revoked or expired are dropped.

After a restart with outstanding labelled state, the store reports
`recovery_incomplete: true` from `status`; AWF must inspect (and revoke as needed) that state and
then call `reconcile` to clear the flag before new dynamic admissions resume.

## Security model

- **Capability isolation.** The control key is AWF-only. Only its SHA-256 digest is retained in
  memory, and comparison is constant-time over fixed-size digests, so neither the value nor its
  length leaks through timing or a memory dump.
- **Listener separation.** The control handler is mounted only on the private control listener and never on the executor-facing data-plane handler; requests to this prefix on the public plane are rejected (the unified mux returns `404`, while proxy delegation returns an access-denied response).
- **Least privilege.** An identity is bound to exactly one repository, one invocation, and the
  closed `github-repository-read-v1` tool set. In proxy mode, only `GET` requests matching a known
  enclave route are delegated; in unified mode, delegated calls are restricted to the `github`
  server and to tools with canonical `owner/repo` arguments.
- **Bounded lifetime.** Identities never outlive `max_identity_ttl`, the invocation deadline, or
  the envelope's `expires_at`.
- **Redacted audit logging.** Run IDs, repository selectors, and identity handles are hashed before
  being written to logs; raw values never reach log lines.

## Related documentation

- [Proxy Mode](PROXY_MODE.md) — including the single-use `issues-read-v1` enclave profile
- [Environment Variables](ENVIRONMENT_VARIABLES.md)
