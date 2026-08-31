package server

import (
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- resolveGuardPolicyForAgent tests ----

func TestResolveGuardPolicyForAgent_NilConfig(t *testing.T) {
	us := &UnifiedServer{cfg: nil}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.NoError(t, err)
	assert.Nil(t, policy)
	assert.Equal(t, legacyPolicySource, source)
}

func TestResolveGuardPolicyForAgent_AgentPoliciesDisabled_FallsBackToServer(t *testing.T) {
	// AgentPoliciesEnabled() is false because Gateway.AgentPolicies is empty,
	// so it should fall back to resolveGuardPolicy even though an AgentPolicyFor
	// lookup would otherwise be attempted.
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{},
		Servers: map[string]*config.ServerConfig{
			"github": {Type: "http"},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.NoError(t, err)
	assert.Nil(t, policy)
	assert.Equal(t, legacyPolicySource, source)
}

func TestResolveGuardPolicyForAgent_AgentPoliciesEnabled_NoPolicyForAgent(t *testing.T) {
	// Agent policies are enabled globally (map is non-empty) but the specific
	// agent ID being queried has no configured policy → falls back to server resolution.
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"other-agent": {Servers: []string{"github"}},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {Type: "http"},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.NoError(t, err)
	assert.Nil(t, policy)
	assert.Equal(t, legacyPolicySource, source)
}

func TestResolveGuardPolicyForAgent_AgentPolicyExists_NoAllowOnly(t *testing.T) {
	// The agent has a configured policy, but it has no AllowOnly guard policy set,
	// so resolution should fall back to server/global resolution rather than using
	// the "agent" source.
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"agent-1": {Servers: []string{"github"}},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {Type: "http"},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.NoError(t, err)
	assert.Nil(t, policy)
	assert.Equal(t, legacyPolicySource, source)
}

func TestResolveGuardPolicyForAgent_AgentAllowOnlyPolicy_ValidPolicy(t *testing.T) {
	// Agent has an AllowOnly policy configured; it should take precedence over
	// any server/global guard policy and be returned with source "agent".
	agentAllowOnly := &config.AllowOnlyPolicy{
		Repos:        "public",
		MinIntegrity: config.IntegrityNone,
	}
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"agent-1": {
					Servers:   []string{"github"},
					AllowOnly: agentAllowOnly,
				},
			},
		},
		// Even though a global guard policy is configured, the per-agent
		// allow-only policy must take precedence.
		GuardPolicy: validAllowOnlyPolicy(),
		Servers: map[string]*config.ServerConfig{
			"github": {Type: "http"},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.Equal(t, "agent", source)
	require.NotNil(t, policy.AllowOnly)
	assert.Equal(t, agentAllowOnly, policy.AllowOnly)
	assert.Nil(t, policy.WriteSink, "agent-derived policy should carry only AllowOnly, no WriteSink")
}

func TestResolveGuardPolicyForAgent_AgentAllowOnlyPolicy_InvalidPolicy(t *testing.T) {
	// The agent's AllowOnly policy is invalid (missing MinIntegrity) → validation
	// should fail and the function should return an error, source empty, and nil policy.
	invalidAllowOnly := &config.AllowOnlyPolicy{
		Repos: "public",
		// MinIntegrity intentionally omitted to make ValidateGuardPolicy fail.
	}
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"agent-1": {
					Servers:   []string{"github"},
					AllowOnly: invalidAllowOnly,
				},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {Type: "http"},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.Error(t, err)
	assert.Nil(t, policy)
	assert.Empty(t, source)
}

func TestResolveGuardPolicyForAgent_AgentPolicyNilInMap(t *testing.T) {
	// The agent ID is present in the map but the value is nil → AgentPolicyFor
	// returns nil, so resolution must fall back to server/global resolution.
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"agent-1": nil,
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {Type: "http"},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.NoError(t, err)
	assert.Nil(t, policy)
	assert.Equal(t, legacyPolicySource, source)
}

func TestResolveGuardPolicyForAgent_AgentPolicyExists_FallsBackToServerPolicy(t *testing.T) {
	// Agent policy exists without AllowOnly, and the server has its own guard
	// policy configured. The fallback to resolveGuardPolicy must return that
	// server-level policy, not the (absent) agent one.
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"agent-1": {Servers: []string{"github"}},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {
				Type: "http",
				GuardPolicies: map[string]interface{}{
					"allow-only": map[string]interface{}{
						"min-integrity": "approved",
						"repos":         []interface{}{"github/gh-aw*"},
					},
				},
			},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("agent-1", "github")

	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.Equal(t, "server", source)
	require.NotNil(t, policy.AllowOnly)
	assert.Equal(t, "approved", policy.AllowOnly.MinIntegrity)
}

func TestResolveGuardPolicyForAgent_EmptyAgentID(t *testing.T) {
	// AgentPoliciesEnabled is true, but the empty agent ID has no configured
	// policy, so lookup falls back to server/global resolution (legacy here).
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"agent-1": {Servers: []string{"github"}, AllowOnly: validAllowOnlyPolicy().AllowOnly},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {Type: "http"},
		},
	}
	us := &UnifiedServer{cfg: cfg}

	policy, source, err := us.resolveGuardPolicyForAgent("", "github")

	require.NoError(t, err)
	assert.Nil(t, policy)
	assert.Equal(t, legacyPolicySource, source)
}
