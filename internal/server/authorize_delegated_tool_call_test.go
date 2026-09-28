package server

import (
	"context"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/delegation"
	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/github/gh-aw-mcpg/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── test helpers ────────────────────────────────────────────────────────────

// newDelegationTestEnvelope builds a valid delegation Envelope scoped to the
// canonical repository used throughout these tests.
func newDelegationTestEnvelope() *delegation.Envelope {
	return &delegation.Envelope{
		RunID:               "run-123",
		EnclaveBackend:      "awf-enclave",
		AllowedRepositories: []string{"github/gh-aw"},
		ToolPolicy:          delegation.ToolPolicyGitHubRepositoryReadV1,
		AllowedSchemaHashes: []string{"sha256:abc"},
		MaxIdentityTTL:      5 * time.Minute,
		ExpiresAt:           time.Now().Add(time.Hour),
	}
}

// newDelegatedExecutorServer builds a UnifiedServer with a live delegation
// Store containing exactly one confirmed identity, scoped to
// github/gh-aw for the closed github-repository-read-v1 tool set. It returns
// the server and the executor bearer token identifying that live session.
func newDelegatedExecutorServer(t *testing.T) (*UnifiedServer, string) {
	t.Helper()
	envelope := newDelegationTestEnvelope()
	store, err := delegation.NewStore(envelope, 1)
	require.NoError(t, err)

	result, err := store.CreateOrConfirm(delegation.CreateOrConfirmRequest{
		RequestCore: delegation.RequestCore{
			RunID:          "run-123",
			EnclaveBackend: "awf-enclave",
			EnclaveEntryID: "entry-1",
			InvocationID:   "inv-1",
			Repository:     "github/gh-aw",
			ToolPolicy:     delegation.ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:     "sha256:abc",
		},
		RequestedTTL:   time.Minute,
		IdempotencyKey: "idem-1",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.ExecutorBearer)

	us := &UnifiedServer{
		guardRegistry: guard.NewRegistry(),
		delegation: &delegation.RuntimeConfig{
			Store: store,
		},
	}
	return us, result.ExecutorBearer
}

// ─── authorizeDelegatedToolCall ──────────────────────────────────────────────

// TestAuthorizeDelegatedToolCall_DelegationDisabled verifies that with no
// delegation runtime configured (us.delegation == nil, the common case for
// non-delegated gateways), the call is a pure no-op: the context is returned
// unmodified and no error occurs, regardless of serverID/tool/args.
func TestAuthorizeDelegatedToolCall_DelegationDisabled(t *testing.T) {
	us := &UnifiedServer{guardRegistry: guard.NewRegistry()}
	ctx := ctxWithSession("some-bearer")

	gotCtx, err := us.authorizeDelegatedToolCall(ctx, "github", "issue_read", map[string]interface{}{
		"owner": "github", "repo": "gh-aw",
	})

	require.NoError(t, err)
	assert.Equal(t, ctx, gotCtx, "context should be returned unchanged when delegation is disabled")
	assert.False(t, delegatedToolAuthorized(gotCtx, "github", "issue_read"))
}

// TestAuthorizeDelegatedToolCall_NonDelegatedSession verifies that when
// delegation is enabled but the calling session's bearer is not a live
// delegated executor (e.g. a normal proxy client), the call is treated as
// not delegated: no error, unmodified context.
func TestAuthorizeDelegatedToolCall_NonDelegatedSession(t *testing.T) {
	us, _ := newDelegatedExecutorServer(t)
	ctx := ctxWithSession("not-a-real-executor-bearer")

	gotCtx, err := us.authorizeDelegatedToolCall(ctx, "github", "issue_read", map[string]interface{}{
		"owner": "github", "repo": "gh-aw",
	})

	require.NoError(t, err)
	assert.Equal(t, ctx, gotCtx)
	assert.False(t, delegatedToolAuthorized(gotCtx, "github", "issue_read"))
}

// TestAuthorizeDelegatedToolCall_UnsupportedServerID verifies that a live
// delegated executor is denied when it targets any backend server other
// than "github" — delegation is scoped exclusively to the github backend.
func TestAuthorizeDelegatedToolCall_UnsupportedServerID(t *testing.T) {
	us, bearer := newDelegatedExecutorServer(t)
	ctx := ctxWithSession(bearer)

	gotCtx, err := us.authorizeDelegatedToolCall(ctx, "slack", "issue_read", map[string]interface{}{
		"owner": "github", "repo": "gh-aw",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `not authorized for server "slack"`)
	assert.Equal(t, ctx, gotCtx, "context should be unmodified on denial")
}

// TestAuthorizeDelegatedToolCall_MissingCanonicalArgs verifies that a live
// delegated executor is denied when the tool call args are missing the
// canonical owner/repo pair required to authorize against the envelope's
// allowed repository scope.
func TestAuthorizeDelegatedToolCall_MissingCanonicalArgs(t *testing.T) {
	us, bearer := newDelegatedExecutorServer(t)
	ctx := ctxWithSession(bearer)

	tests := []struct {
		name string
		args interface{}
	}{
		{"nil args", nil},
		{"non-map args", "not-a-map"},
		{"missing owner", map[string]interface{}{"repo": "gh-aw"}},
		{"missing repo", map[string]interface{}{"owner": "github"}},
		{"non-string owner", map[string]interface{}{"owner": 42, "repo": "gh-aw"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCtx, err := us.authorizeDelegatedToolCall(ctx, "github", "issue_read", tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "requires canonical owner/repo arguments")
			assert.Equal(t, ctx, gotCtx)
		})
	}
}

// TestAuthorizeDelegatedToolCall_NonDelegatedTool verifies that a tool name
// outside the closed github-repository-read-v1 set (e.g. a write tool) is
// denied even with well-formed owner/repo arguments, because
// delegatedToolRepository requires the tool itself to be in the delegated
// set before it will extract a repository at all.
func TestAuthorizeDelegatedToolCall_NonDelegatedTool(t *testing.T) {
	us, bearer := newDelegatedExecutorServer(t)
	ctx := ctxWithSession(bearer)

	gotCtx, err := us.authorizeDelegatedToolCall(ctx, "github", "create_issue", map[string]interface{}{
		"owner": "github", "repo": "gh-aw",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires canonical owner/repo arguments")
	assert.Equal(t, ctx, gotCtx)
}

// TestAuthorizeDelegatedToolCall_RepositoryOutsideEnvelopeScope verifies that
// a live delegated executor is denied when the well-formed canonical
// owner/repo does not match any repository authorized by its identity
// (Store.AuthorizeExecutor enforces the binding, not this function, but the
// error must still propagate through unmodified).
func TestAuthorizeDelegatedToolCall_RepositoryOutsideEnvelopeScope(t *testing.T) {
	us, bearer := newDelegatedExecutorServer(t)
	ctx := ctxWithSession(bearer)

	gotCtx, err := us.authorizeDelegatedToolCall(ctx, "github", "issue_read", map[string]interface{}{
		"owner": "someone-else", "repo": "other-repo",
	})

	require.Error(t, err)
	assert.Equal(t, ctx, gotCtx)
	assert.False(t, delegatedToolAuthorized(gotCtx, "github", "issue_read"))
}

// TestAuthorizeDelegatedToolCall_Success verifies the full happy path: a live
// delegated executor calling an in-scope tool against its bound repository
// is authorized, and the returned context carries all three provenance
// markers set by the function — the delegation-scoped agent ID, the enclave
// session marker, and the per-call tool authorization record consumed by
// delegatedToolAuthorized.
func TestAuthorizeDelegatedToolCall_Success(t *testing.T) {
	us, bearer := newDelegatedExecutorServer(t)
	ctx := ctxWithSession(bearer)

	gotCtx, err := us.authorizeDelegatedToolCall(ctx, "github", "issue_read", map[string]interface{}{
		"owner": "github", "repo": "gh-aw",
	})

	require.NoError(t, err)
	require.NotEqual(t, ctx, gotCtx, "context should be enriched on successful authorization")

	agentID := guard.GetAgentIDFromContext(gotCtx)
	assert.Contains(t, agentID, "delegation:", "agent ID should carry the delegation-scoped handle prefix")

	assert.True(t, mcp.IsEnclaveSession(gotCtx), "successful delegated authorization must mark the context as an enclave session")

	assert.True(t, delegatedToolAuthorized(gotCtx, "github", "issue_read"))
	// A different serverID/toolName pair must not be considered authorized by
	// this same context value, even though a valid authorization is present.
	assert.False(t, delegatedToolAuthorized(gotCtx, "github", "list_issues"))
	assert.False(t, delegatedToolAuthorized(gotCtx, "slack", "issue_read"))
}

// TestAuthorizeDelegatedToolCall_ListIssuesAlsoDelegated verifies that
// list_issues — the second member of the closed delegated tool set — is also
// accepted by delegatedToolRepository and authorized end-to-end.
func TestAuthorizeDelegatedToolCall_ListIssuesAlsoDelegated(t *testing.T) {
	us, bearer := newDelegatedExecutorServer(t)
	ctx := ctxWithSession(bearer)

	gotCtx, err := us.authorizeDelegatedToolCall(ctx, "github", "list_issues", map[string]interface{}{
		"owner": "github", "repo": "gh-aw",
	})

	require.NoError(t, err)
	assert.True(t, delegatedToolAuthorized(gotCtx, "github", "list_issues"))
}

// ─── isDelegatedExecutorSession ──────────────────────────────────────────────

// TestIsDelegatedExecutorSession_DelegationDisabled verifies the fast-path
// false result when no delegation runtime is configured.
func TestIsDelegatedExecutorSession_DelegationDisabled(t *testing.T) {
	us := &UnifiedServer{guardRegistry: guard.NewRegistry()}
	assert.False(t, us.isDelegatedExecutorSession("anything"))
}

// TestIsDelegatedExecutorSession_LiveBearer verifies that a live executor
// bearer from a confirmed identity is recognized as a delegated session.
func TestIsDelegatedExecutorSession_LiveBearer(t *testing.T) {
	us, bearer := newDelegatedExecutorServer(t)
	assert.True(t, us.isDelegatedExecutorSession(bearer))
}

// TestIsDelegatedExecutorSession_UnknownBearer verifies that an
// unrecognized session ID is not treated as a delegated executor session
// even when delegation is enabled.
func TestIsDelegatedExecutorSession_UnknownBearer(t *testing.T) {
	us, _ := newDelegatedExecutorServer(t)
	assert.False(t, us.isDelegatedExecutorSession("some-other-session-id"))
}

// ─── delegatedToolRepository ─────────────────────────────────────────────────

// TestDelegatedToolRepository covers every branch of the helper: the tool
// gate, the args-type assertion, and the owner/repo presence/type checks,
// plus the terminal canonical-selector validation delegated to reposelector.
func TestDelegatedToolRepository(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		args     interface{}
		wantRepo string
		wantOK   bool
	}{
		{
			name:   "non-delegated tool rejected regardless of args",
			tool:   "create_issue",
			args:   map[string]interface{}{"owner": "github", "repo": "gh-aw"},
			wantOK: false,
		},
		{
			name:   "non-map args rejected",
			tool:   "issue_read",
			args:   "owner=github,repo=gh-aw",
			wantOK: false,
		},
		{
			name:   "nil args rejected",
			tool:   "issue_read",
			args:   nil,
			wantOK: false,
		},
		{
			name:   "missing owner rejected",
			tool:   "issue_read",
			args:   map[string]interface{}{"repo": "gh-aw"},
			wantOK: false,
		},
		{
			name:   "missing repo rejected",
			tool:   "issue_read",
			args:   map[string]interface{}{"owner": "github"},
			wantOK: false,
		},
		{
			name:   "non-string owner rejected",
			tool:   "issue_read",
			args:   map[string]interface{}{"owner": 1, "repo": "gh-aw"},
			wantOK: false,
		},
		{
			name:   "non-string repo rejected",
			tool:   "issue_read",
			args:   map[string]interface{}{"owner": "github", "repo": 1},
			wantOK: false,
		},
		{
			name:     "valid canonical owner/repo accepted for issue_read",
			tool:     "issue_read",
			args:     map[string]interface{}{"owner": "github", "repo": "gh-aw"},
			wantRepo: "github/gh-aw",
			wantOK:   true,
		},
		{
			name:     "valid canonical owner/repo accepted for list_issues",
			tool:     "list_issues",
			args:     map[string]interface{}{"owner": "github", "repo": "gh-aw"},
			wantRepo: "github/gh-aw",
			wantOK:   true,
		},
		{
			name:   "non-canonical selector rejected (path traversal attempt)",
			tool:   "issue_read",
			args:   map[string]interface{}{"owner": "../etc", "repo": "passwd"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, ok := delegatedToolRepository(tt.tool, tt.args)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantRepo, repo)
			}
		})
	}
}

// ─── delegatedToolAuthorized ─────────────────────────────────────────────────

// TestDelegatedToolAuthorized_NoAuthorizationInContext verifies that a
// context with no prior authorizeDelegatedToolCall marker is never
// considered authorized.
func TestDelegatedToolAuthorized_NoAuthorizationInContext(t *testing.T) {
	assert.False(t, delegatedToolAuthorized(context.Background(), "github", "issue_read"))
}
