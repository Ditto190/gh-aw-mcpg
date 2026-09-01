package server

import (
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateAgentPolicyTools exercises UnifiedServer.validateAgentPolicyTools
// across all of its branches: no config, no gateway, no policies configured,
// policies without a per-server tool allowlist, wildcard allowlists, known
// tools, unknown tools, nil policies, and multiple agents where sorting of
// agent IDs determines which error surfaces first.
func TestValidateAgentPolicyTools(t *testing.T) {
	advertised := []backendToolDefinition{
		{Name: "search_code"},
		{Name: "get_file_contents"},
	}

	tests := []struct {
		name       string
		us         *UnifiedServer
		serverID   string
		tools      []backendToolDefinition
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:     "nil config returns nil",
			us:       &UnifiedServer{cfg: nil},
			serverID: "github",
			tools:    advertised,
			wantErr:  false,
		},
		{
			name:     "nil gateway returns nil",
			us:       &UnifiedServer{cfg: &config.Config{}},
			serverID: "github",
			tools:    advertised,
			wantErr:  false,
		},
		{
			name: "no agent policies configured returns nil",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{},
			}},
			serverID: "github",
			tools:    advertised,
			wantErr:  false,
		},
		{
			name: "nil policy entry is skipped",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"agent-a": nil,
					},
				},
			}},
			serverID: "github",
			tools:    advertised,
			wantErr:  false,
		},
		{
			name: "server not configured in policy tools is skipped",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"agent-a": {
							Servers: []string{"fetch"},
							// Tools has no entry for "github" -> configured==false -> skip.
						},
					},
				},
			}},
			serverID: "github",
			tools:    advertised,
			wantErr:  false,
		},
		{
			name: "wildcard tool entry is always allowed",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"agent-a": {
							Servers: []string{"github"},
							Tools:   map[string][]string{"github": {"*"}},
						},
					},
				},
			}},
			serverID: "github",
			tools:    advertised,
			wantErr:  false,
		},
		{
			name: "known tool passes validation",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"agent-a": {
							Servers: []string{"github"},
							Tools:   map[string][]string{"github": {"search_code"}},
						},
					},
				},
			}},
			serverID: "github",
			tools:    advertised,
			wantErr:  false,
		},
		{
			name: "unknown tool causes validation error",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"agent-a": {
							Servers: []string{"github"},
							Tools:   map[string][]string{"github": {"delete_repo"}},
						},
					},
				},
			}},
			serverID:   "github",
			tools:      advertised,
			wantErr:    true,
			wantErrMsg: `agent policy agent:[a-f0-9]+ references unknown tool "delete_repo" on server "github"`,
		},
		{
			name: "multiple agents sorted, first alphabetical failure surfaces",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"zeta": {
							Servers: []string{"github"},
							Tools:   map[string][]string{"github": {"search_code"}},
						},
						"alpha": {
							Servers: []string{"github"},
							Tools:   map[string][]string{"github": {"nonexistent_tool"}},
						},
					},
				},
			}},
			serverID: "github",
			tools:    advertised,
			wantErr:  true,
		},
		{
			name: "empty advertised tools with wildcard still passes",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"agent-a": {
							Servers: []string{"github"},
							Tools:   map[string][]string{"github": {"*"}},
						},
					},
				},
			}},
			serverID: "github",
			tools:    nil,
			wantErr:  false,
		},
		{
			name: "empty advertised tools with named tool fails",
			us: &UnifiedServer{cfg: &config.Config{
				Gateway: &config.GatewayConfig{
					AgentPolicies: map[string]*config.AgentPolicy{
						"agent-a": {
							Servers: []string{"github"},
							Tools:   map[string][]string{"github": {"search_code"}},
						},
					},
				},
			}},
			serverID: "github",
			tools:    nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.us.validateAgentPolicyTools(tt.serverID, tt.tools)
			if tt.wantErr {
				require.Error(t, err)
				var validationErr *agentPolicyToolValidationError
				assert.ErrorAs(t, err, &validationErr)
				if tt.wantErrMsg != "" {
					assert.Regexp(t, tt.wantErrMsg, err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateAgentPolicyTools_AlphabeticalOrdering verifies that when multiple
// agent policies would each fail validation, the error returned corresponds to
// the alphabetically-first agent ID, since validateAgentPolicyTools sorts agent
// IDs before iterating.
func TestValidateAgentPolicyTools_AlphabeticalOrdering(t *testing.T) {
	us := &UnifiedServer{cfg: &config.Config{
		Gateway: &config.GatewayConfig{
			AgentPolicies: map[string]*config.AgentPolicy{
				"zeta": {
					Servers: []string{"github"},
					Tools:   map[string][]string{"github": {"zeta_unknown_tool"}},
				},
				"alpha": {
					Servers: []string{"github"},
					Tools:   map[string][]string{"github": {"alpha_unknown_tool"}},
				},
			},
		},
	}}

	err := us.validateAgentPolicyTools("github", []backendToolDefinition{{Name: "search_code"}})
	require.Error(t, err)
	// "alpha" sorts before "zeta", so its unknown tool should be the one reported.
	assert.Contains(t, err.Error(), "alpha_unknown_tool")
}
