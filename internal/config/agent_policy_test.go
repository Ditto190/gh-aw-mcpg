package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Unit tests for the AgentPolicy access-decision helpers ---

func TestAgentPolicy_AllowsServer(t *testing.T) {
	p := &AgentPolicy{Servers: []string{"github", "fetch"}}
	assert.True(t, p.AllowsServer("github"))
	assert.True(t, p.AllowsServer("fetch"))
	assert.False(t, p.AllowsServer("slack"))
	assert.False(t, p.AllowsServer(""))

	// nil policy denies (fail-closed).
	var nilPolicy *AgentPolicy
	assert.False(t, nilPolicy.AllowsServer("github"))
}

func TestAgentPolicy_AllowsTool(t *testing.T) {
	p := &AgentPolicy{
		Servers: []string{"github", "fetch"},
		Tools: map[string][]string{
			"github": {"search_code", "get_file_contents"},
			"fetch":  {"*"},
		},
	}

	// Server not permitted -> denied regardless of tool.
	assert.False(t, p.AllowsTool("slack", "send_message"))

	// Explicit tool allowlist.
	assert.True(t, p.AllowsTool("github", "search_code"))
	assert.False(t, p.AllowsTool("github", "delete_repo"))

	// Wildcard allows all tools on that server.
	assert.True(t, p.AllowsTool("fetch", "anything"))

	// Server permitted but no per-server tool list -> all tools allowed.
	p2 := &AgentPolicy{Servers: []string{"github"}}
	assert.True(t, p2.AllowsTool("github", "any_tool"))

	// An explicitly configured empty allowlist denies every tool.
	p3 := &AgentPolicy{Servers: []string{"github"}, Tools: map[string][]string{"github": {}}}
	assert.False(t, p3.AllowsTool("github", "any_tool"))
}

func TestValidateSingleAgentPolicy_RejectsSurroundingWhitespace(t *testing.T) {
	servers := map[string]*ServerConfig{"github": {}}
	tests := []struct {
		name   string
		policy *AgentPolicy
	}{
		{
			name:   "server entry",
			policy: &AgentPolicy{Servers: []string{" github "}},
		},
		{
			name: "tools server key",
			policy: &AgentPolicy{
				Servers: []string{"github"},
				Tools:   map[string][]string{" github ": {"search_code"}},
			},
		},
		{
			name: "tool entry",
			policy: &AgentPolicy{
				Servers: []string{"github"},
				Tools:   map[string][]string{"github": {" search_code "}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSingleAgentPolicy("secret-agent-id", tt.policy, servers)
			require.Error(t, err)
			assert.ErrorContains(t, err, "surrounding whitespace")
			assert.NotContains(t, err.Error(), "secret-agent-id")
		})
	}
}

// TestValidateSingleAgentPolicy_ErrorPaths covers the remaining validation
// failure branches in validateSingleAgentPolicy that were previously
// uncovered: empty server/tool entries and duplicate server/tool references.
func TestValidateSingleAgentPolicy_ErrorPaths(t *testing.T) {
	servers := map[string]*ServerConfig{"github": {}, "fetch": {}}

	tests := []struct {
		name        string
		policy      *AgentPolicy
		errContains string
	}{
		{
			name:        "empty server entry",
			policy:      &AgentPolicy{Servers: []string{""}},
			errContains: "must be non-empty strings",
		},
		{
			name:        "unknown server reference",
			policy:      &AgentPolicy{Servers: []string{"unknown-server"}},
			errContains: "references unknown server",
		},
		{
			name:        "duplicate server entry",
			policy:      &AgentPolicy{Servers: []string{"github", "github"}},
			errContains: "must not contain duplicate server",
		},
		{
			name: "tools reference server not in policy's servers list",
			policy: &AgentPolicy{
				Servers: []string{"github"},
				Tools:   map[string][]string{"fetch": {"fetch_url"}},
			},
			errContains: "not in the policy's servers list",
		},
		{
			name: "empty tool entry",
			policy: &AgentPolicy{
				Servers: []string{"github"},
				Tools:   map[string][]string{"github": {""}},
			},
			errContains: "must be non-empty strings",
		},
		{
			name: "duplicate tool entry",
			policy: &AgentPolicy{
				Servers: []string{"github"},
				Tools:   map[string][]string{"github": {"search_code", "search_code"}},
			},
			errContains: "must not contain duplicate tool",
		},
		{
			name: "invalid allow-only policy",
			policy: &AgentPolicy{
				Servers:   []string{"github"},
				AllowOnly: &AllowOnlyPolicy{MinIntegrity: "not-a-real-level"},
			},
			errContains: "allow-only is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSingleAgentPolicy("secret-agent-id", tt.policy, servers)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.errContains)
			assert.NotContains(t, err.Error(), "secret-agent-id")
		})
	}
}

func TestValidateSingleAgentPolicy_ValidPolicySucceeds(t *testing.T) {
	servers := map[string]*ServerConfig{"github": {}, "fetch": {}}
	policy := &AgentPolicy{
		Servers: []string{"github", "fetch"},
		Tools: map[string][]string{
			"github": {"search_code", "get_file_contents"},
			"fetch":  {"*"},
		},
	}

	err := validateSingleAgentPolicy("agent-1", policy, servers)
	assert.NoError(t, err)
}

// --- Config accessor tests ---

func TestConfig_AgentPolicyAccessors(t *testing.T) {
	cfg := &Config{Gateway: &GatewayConfig{
		AgentPolicies: map[string]*AgentPolicy{
			"a": {Servers: []string{"github"}, AllowOnly: &AllowOnlyPolicy{Repos: "public"}},
		},
	}}
	assert.True(t, cfg.AgentPoliciesEnabled())
	assert.NotNil(t, cfg.AgentPolicyFor("a"))
	assert.Nil(t, cfg.AgentPolicyFor("missing"))
	assert.True(t, cfg.HasAgentAllowOnlyPolicies())

	// No policies -> disabled, backward compatible.
	empty := &Config{Gateway: &GatewayConfig{}}
	assert.False(t, empty.AgentPoliciesEnabled())
	assert.Nil(t, empty.AgentPolicyFor("a"))
	assert.False(t, empty.HasAgentAllowOnlyPolicies())

	// Nil config is safe.
	var nilCfg *Config
	assert.False(t, nilCfg.AgentPoliciesEnabled())
	assert.Nil(t, nilCfg.AgentPolicyFor("a"))
	assert.False(t, nilCfg.HasAgentAllowOnlyPolicies())
}

// --- TOML loading + fail-closed validation tests ---

func TestLoadFromFile_AgentPolicies(t *testing.T) {
	const servers = `
[servers.github]
command = "docker"
args = ["run", "--rm", "-i", "ghcr.io/github/github-mcp-server:latest"]

[servers.fetch]
command = "docker"
args = ["run", "--rm", "-i", "ghcr.io/example/fetch:latest"]
`

	tests := []struct {
		name    string
		gateway string
		wantErr string
	}{
		{
			name: "singular agent with policy is valid",
			gateway: `agent_id = "solo"

[gateway.agent_policies.solo]
servers = ["github"]

[gateway.agent_policies.solo.tools]
github = ["search_code"]
`,
		},
		{
			name: "multi-agent with policy for each is valid",
			gateway: `agent_ids = ["a", "b"]

[gateway.agent_policies.a]
servers = ["github"]

[gateway.agent_policies.b]
servers = ["fetch"]
`,
		},
		{
			name: "multi-agent missing a policy fails closed",
			gateway: `agent_ids = ["a", "b"]

[gateway.agent_policies.a]
servers = ["github"]
`,
			wantErr: "must define a policy for every configured agent ID",
		},
		{
			name:    "multi-agent without policies fails closed",
			gateway: `agent_ids = ["a", "b"]`,
			wantErr: "must define a policy for every configured agent ID",
		},
		{
			name: "unknown policy ID rejected",
			gateway: `agent_id = "solo"

[gateway.agent_policies.ghost]
servers = ["github"]
`,
			wantErr: "unknown agent ID",
		},
		{
			name: "server reference must exist",
			gateway: `agent_id = "solo"

[gateway.agent_policies.solo]
servers = ["nope"]
`,
			wantErr: "references unknown server",
		},
		{
			name: "tools key must be in servers",
			gateway: `agent_id = "solo"

[gateway.agent_policies.solo]
servers = ["github"]

[gateway.agent_policies.solo.tools]
fetch = ["x"]
`,
			wantErr: "not in the policy's servers list",
		},
		{
			name: "duplicate agent IDs rejected",
			gateway: `agent_ids = ["dup", "dup"]

[gateway.agent_policies.dup]
servers = ["github"]
`,
			wantErr: "duplicate agent ID",
		},
		{
			name: "per-agent allow-only validated",
			gateway: `agent_id = "solo"

[gateway.agent_policies.solo]
servers = ["github"]

[gateway.agent_policies.solo.allow-only]
repos = "not-a-valid-value"
`,
			wantErr: "allow-only",
		},
		{
			name: "valid per-agent allow-only",
			gateway: `agent_id = "solo"

[gateway.agent_policies.solo]
servers = ["github"]

[gateway.agent_policies.solo.allow-only]
repos = "public"
min-integrity = "none"
`,
		},
		{
			name: "no agent policies preserves full access (singular compat)",
			gateway: `agent_id = "solo"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempTOML(t, "[gateway]\n"+tt.gateway+servers)
			cfg, err := LoadFromFile(path)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg.Gateway)
		})
	}
}

// --- JSON stdin path validation ---

func TestLoadFromStdin_AgentPolicies(t *testing.T) {
	tests := []struct {
		name      string
		gateway   string
		shouldErr bool
		wantErr   string
	}{
		{
			name:    "valid single agent policy",
			gateway: `"agentId": "solo", "agentPolicies": {"solo": {"servers": ["github"], "tools": {"github": ["search_code"]}}}`,
		},
		{
			name:    "valid multiple agent policies",
			gateway: `"agentIds": ["a", "b"], "agentPolicies": {"a": {"servers": ["github"], "tools": {"github": ["search_code"]}, "allow-only": {"repos": "public", "min-integrity": "none"}}, "b": {"servers": ["github"]}}`,
		},
		{
			name:      "multi-agent missing policy fails closed",
			gateway:   `"agentIds": ["a", "b"], "agentPolicies": {"a": {"servers": ["github"]}}`,
			shouldErr: true,
			wantErr:   "must define a policy for every configured agent ID",
		},
		{
			name:      "multi-agent without policies fails closed",
			gateway:   `"agentIds": ["a", "b"]`,
			shouldErr: true,
			wantErr:   "must define a policy for every configured agent ID",
		},
		{
			name:      "unknown policy ID rejected",
			gateway:   `"agentId": "solo", "agentPolicies": {"ghost": {"servers": ["github"]}}`,
			shouldErr: true,
			wantErr:   "unknown agent ID",
		},
		{
			name:      "server reference must exist",
			gateway:   `"agentId": "solo", "agentPolicies": {"solo": {"servers": ["nope"]}}`,
			shouldErr: true,
			wantErr:   "references unknown server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{
				"mcpServers": {"github": {"type": "stdio", "container": "ghcr.io/github/github-mcp-server:latest"}},
				"gateway": {"port": 3000, "domain": "localhost", ` + tt.gateway + `}
			}`
			readEnd, writeEnd, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = readEnd.Close() })

			oldStdin := os.Stdin
			os.Stdin = readEnd
			t.Cleanup(func() { os.Stdin = oldStdin })
			_, err = writeEnd.WriteString(input)
			require.NoError(t, err)
			require.NoError(t, writeEnd.Close())

			cfg, err := LoadFromStdin()
			if tt.shouldErr {
				require.Error(t, err)
				if tt.wantErr != "" {
					require.ErrorContains(t, err, tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg.Gateway)
			assert.True(t, cfg.AgentPoliciesEnabled())
		})
	}
}

func TestAgentPolicies_TOMLAndStdinJSONParity(t *testing.T) {
	tomlPath := writeTempTOML(t, `[gateway]
agent_ids = ["primary", "enclave"]

[gateway.agent_policies.primary]
servers = ["github"]

[gateway.agent_policies.primary.tools]
github = ["search_code"]

[gateway.agent_policies.enclave]
servers = ["github"]

[gateway.agent_policies.enclave.allow-only]
repos = ["github/gh-aw"]
min-integrity = "approved"

[servers.github]
command = "docker"
args = ["run", "--rm", "-i", "ghcr.io/github/github-mcp-server:latest"]
`)
	tomlConfig, err := LoadFromFile(tomlPath)
	require.NoError(t, err)

	input := `{
		"mcpServers": {
			"github": {
				"type": "stdio",
				"container": "ghcr.io/github/github-mcp-server:latest"
			}
		},
		"gateway": {
			"port": 3000,
			"domain": "localhost",
			"agentIds": ["primary", "enclave"],
			"agentPolicies": {
				"primary": {
					"servers": ["github"],
					"tools": {"github": ["search_code"]}
				},
				"enclave": {
					"servers": ["github"],
					"allow-only": {
						"repos": ["github/gh-aw"],
						"min-integrity": "approved"
					}
				}
			}
		}
	}`
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = readEnd.Close() })
	oldStdin := os.Stdin
	os.Stdin = readEnd
	t.Cleanup(func() { os.Stdin = oldStdin })
	_, err = writeEnd.WriteString(input)
	require.NoError(t, err)
	require.NoError(t, writeEnd.Close())

	jsonConfig, err := LoadFromStdin()
	require.NoError(t, err)
	assert.Equal(t, tomlConfig.Gateway.AgentIDs, jsonConfig.Gateway.AgentIDs)
	assert.Equal(t, tomlConfig.Gateway.AgentPolicies, jsonConfig.Gateway.AgentPolicies)
}
