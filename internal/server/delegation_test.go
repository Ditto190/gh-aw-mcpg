package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/delegation"
	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/github/gh-aw-mcpg/internal/sanitize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUnifiedDelegationConfig(t *testing.T) (*delegation.RuntimeConfig, delegation.CreateOrConfirmRequest) {
	t.Helper()
	envelope := &delegation.Envelope{
		RunID:               "run-1",
		EnclaveBackend:      "awf-enclave",
		AllowedRepositories: []string{"github/gh-aw"},
		ToolPolicy:          delegation.ToolPolicyGitHubRepositoryReadV1,
		AllowedSchemaHashes: []string{"sha256:test"},
		MaxIdentityTTL:      120 * time.Second,
		ExpiresAt:           time.Now().Add(time.Hour),
	}
	store, err := delegation.NewStore(envelope, 1)
	require.NoError(t, err)
	capability, err := delegation.NewControlCapability("control-capability-key-32-bytes!!")
	require.NoError(t, err)
	return &delegation.RuntimeConfig{
			Store:             store,
			Capability:        capability,
			StatePath:         t.TempDir() + "/state.json",
			ControlListenAddr: "127.0.0.1:0",
		}, delegation.CreateOrConfirmRequest{
			RunID:          "run-1",
			EnclaveBackend: "awf-enclave",
			EnclaveEntryID: "entry-1",
			InvocationID:   "inv-1",
			Repository:     "github/gh-aw",
			ToolPolicy:     delegation.ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:     "sha256:test",
			RequestedTTL:   time.Minute,
			IdempotencyKey: "key-1",
		}
}

func TestDelegatedAuthAdmitsOnlyLiveExecutorBearer(t *testing.T) {
	delegationConfig, createReq := newUnifiedDelegationConfig(t)
	created, err := delegationConfig.Store.CreateOrConfirm(createReq)
	require.NoError(t, err)

	us := &UnifiedServer{delegation: delegationConfig}
	called := false
	handler := applyAuthIfConfiguredWithDelegation([]string{"gateway-key"}, us.isDelegatedExecutorAuth, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodPost, "/mcp", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", created.ExecutorBearer)
	rec := httptest.NewRecorder()
	handler(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, called)

	require.NoError(t, delegationConfig.Store.Revoke(created.Handle))
	called = false
	rec = httptest.NewRecorder()
	handler(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, called)
}

func TestDelegatedAuthWithoutGatewayKeyRequiresLiveBearer(t *testing.T) {
	delegationConfig, createReq := newUnifiedDelegationConfig(t)
	created, err := delegationConfig.Store.CreateOrConfirm(createReq)
	require.NoError(t, err)

	us := &UnifiedServer{delegation: delegationConfig}
	called := false
	handler := applyAuthIfConfiguredWithDelegation(nil, us.isDelegatedExecutorAuth, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req, err := http.NewRequest(http.MethodPost, "/mcp", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", created.ExecutorBearer)
	req.Header.Set("X-Agent-ID", "attacker-agent")
	rec := httptest.NewRecorder()
	handler(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, called)

	require.NoError(t, delegationConfig.Store.Revoke(created.Handle))
	called = false
	rec = httptest.NewRecorder()
	handler(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, called)
}

func TestRejectDelegatedNonToolMethodsDeniesUnauthorizedMethod(t *testing.T) {
	delegationConfig, createReq := newUnifiedDelegationConfig(t)
	created, err := delegationConfig.Store.CreateOrConfirm(createReq)
	require.NoError(t, err)

	us := &UnifiedServer{delegation: delegationConfig}
	called := false
	handler := us.rejectDelegatedNonToolMethods(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	body := `{"jsonrpc":"2.0","id":1,"method":"prompts/get"}`
	req, err := http.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", created.ExecutorBearer)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.False(t, called)
}

func TestRejectDelegatedNonToolMethodsDeniesBatchedUnauthorizedMethod(t *testing.T) {
	delegationConfig, createReq := newUnifiedDelegationConfig(t)
	created, err := delegationConfig.Store.CreateOrConfirm(createReq)
	require.NoError(t, err)

	us := &UnifiedServer{delegation: delegationConfig}

	testCases := []struct {
		name string
		body string
	}{
		{
			name: "batched prompts/list",
			body: `[{"jsonrpc":"2.0","id":1,"method":"tools/call"},{"jsonrpc":"2.0","id":2,"method":"prompts/list"}]`,
		},
		{
			name: "batched prompts/get",
			body: `[{"jsonrpc":"2.0","id":1,"method":"tools/list"},{"jsonrpc":"2.0","id":2,"method":"prompts/get"}]`,
		},
		{
			name: "malformed envelope",
			body: `{"jsonrpc":"2.0","id":1,`,
		},
		{
			name: "batch element missing method",
			body: `[{"jsonrpc":"2.0","id":1,"method":"tools/list"},{"jsonrpc":"2.0","id":2}]`,
		},
		{
			name: "empty batch",
			body: `[]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := us.rejectDelegatedNonToolMethods(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			req, err := http.NewRequest(http.MethodPost, "/mcp", strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Authorization", created.ExecutorBearer)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.False(t, called)
		})
	}
}

func TestRejectDelegatedNonToolMethodsAllowsBatchedToolMethods(t *testing.T) {
	delegationConfig, createReq := newUnifiedDelegationConfig(t)
	created, err := delegationConfig.Store.CreateOrConfirm(createReq)
	require.NoError(t, err)

	us := &UnifiedServer{delegation: delegationConfig}
	called := false
	handler := us.rejectDelegatedNonToolMethods(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	body := `[{"jsonrpc":"2.0","id":1,"method":"tools/list"},{"jsonrpc":"2.0","id":2,"method":"tools/call"}]`
	req, err := http.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", created.ExecutorBearer)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, called)
}

func TestUnifiedDelegationAuthorizesExactGitHubRepositoryTool(t *testing.T) {
	previousRedaction := sanitize.PrivateSelectorRedactionEnabled()
	t.Cleanup(func() { sanitize.SetPrivateSelectorRedaction(previousRedaction) })

	delegationConfig, createReq := newUnifiedDelegationConfig(t)
	created, err := delegationConfig.Store.CreateOrConfirm(createReq)
	require.NoError(t, err)

	backend := newBackendWithToolResponse(t, "list_issues", defaultToolResponse)
	defer backend.Close()

	guardName := "unified-delegation-guard"
	guard.RegisterGuardType(guardName, func() (guard.Guard, error) { return &difcTestGuard{}, nil })
	cfg := &config.Config{
		DIFCMode: "filter",
		Gateway: &config.GatewayConfig{
			AgentID: "enclave-agent",
			AgentPolicies: map[string]*config.AgentPolicy{
				"enclave-agent": {Servers: []string{"github"}, Tools: map[string][]string{"github": {"list_issues"}}},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {
				Type:  "http",
				URL:   backend.URL,
				Guard: guardName,
				GuardPolicies: map[string]interface{}{
					"allow-only": map[string]interface{}{"repos": "public", "min-integrity": "none"},
				},
			},
		},
		Guards: map[string]*config.GuardConfig{
			guardName: {Type: guardName},
		},
		GuardPolicy: &config.GuardPolicy{
			AllowOnly: &config.AllowOnlyPolicy{Repos: "public", MinIntegrity: config.IntegrityNone},
		},
		GuardPolicySource: "cli",
		Delegation:        delegationConfig,
	}
	us, err := NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	defer us.Close()

	result, _, err := us.callBackendTool(callCtx(created.ExecutorBearer), "github", "list_issues", map[string]interface{}{
		"owner": "github",
		"repo":  "gh-aw",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	result, _, err = us.callBackendTool(callCtx(created.ExecutorBearer), "github", "list_issues", map[string]interface{}{
		"owner": "github",
		"repo":  "gh-aw-firewall",
	})
	require.Error(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	result, _, err = us.callBackendTool(callCtx(created.ExecutorBearer), "github", "search_issues", map[string]interface{}{
		"owner": "github",
		"repo":  "gh-aw",
	})
	require.Error(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}
