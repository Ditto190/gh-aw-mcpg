package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGatewaySessionTimeoutFromEnv(t *testing.T) {
	t.Run("reads duration from env", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_SESSION_TIMEOUT", "2h")
		assert.Equal(t, 2*time.Hour, GetGatewaySessionTimeoutFromEnv())
	})

	t.Run("defaults to 6h", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_SESSION_TIMEOUT", "")
		assert.Equal(t, 6*time.Hour, GetGatewaySessionTimeoutFromEnv())
	})
}

func TestResolveGuardPolicyOverride(t *testing.T) {
	t.Run("no override", func(t *testing.T) {
		policy, source, err := ResolveGuardPolicyOverride(false, "", false, "", "", "")
		require.NoError(t, err)
		assert.Nil(t, policy)
		assert.Empty(t, source)
	})

	t.Run("cli policy json has highest priority", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_GUARD_POLICY_JSON", `{"allow-only":{"repos":"public","min-integrity":"none"}}`)

		policy, source, err := ResolveGuardPolicyOverride(
			true,
			`{"allow-only":{"repos":["myorg/*"],"min-integrity":"approved"}}`,
			false,
			"",
			"",
			"",
		)

		require.NoError(t, err)
		require.NotNil(t, policy)
		assert.Equal(t, "cli", source)
		assert.Equal(t, "approved", policy.AllowOnly.MinIntegrity)
	})

	t.Run("env policy json has priority over env allowonly vars", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_GUARD_POLICY_JSON", `{"allow-only":{"repos":"public","min-integrity":"none"}}`)
		t.Setenv("MCP_GATEWAY_ALLOWONLY_SCOPE_OWNER", "myorg")
		t.Setenv("MCP_GATEWAY_ALLOWONLY_MIN_INTEGRITY", "approved")

		policy, source, err := ResolveGuardPolicyOverride(false, "", false, "", "", "")
		require.NoError(t, err)
		require.NotNil(t, policy)
		assert.Equal(t, "env", source)
		assert.Equal(t, "none", policy.AllowOnly.MinIntegrity)
		assert.Equal(t, "public", policy.AllowOnly.Repos)
	})

	t.Run("env allowonly vars are used when guard policy json env is unset", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_ALLOWONLY_SCOPE_PUBLIC", "true")
		t.Setenv("MCP_GATEWAY_ALLOWONLY_MIN_INTEGRITY", "merged")

		policy, source, err := ResolveGuardPolicyOverride(false, "", false, "", "", "")
		require.NoError(t, err)
		require.NotNil(t, policy)
		assert.Equal(t, "env", source)
		assert.Equal(t, "public", policy.AllowOnly.Repos)
		assert.Equal(t, "merged", policy.AllowOnly.MinIntegrity)
	})
}
