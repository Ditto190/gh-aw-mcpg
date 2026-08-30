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
					assert.ErrorContains(t, err, tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg.Gateway)
			assert.True(t, cfg.AgentPoliciesEnabled())
		})
	}
}
