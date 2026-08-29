package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromFile_AgentIDs(t *testing.T) {
	tests := []struct {
		name    string
		gateway string
		wantErr string
	}{
		{
			name:    "valid plural IDs",
			gateway: `agent_ids = ["primary-agent", "enclave-agent"]`,
		},
		{
			name:    "empty plural IDs",
			gateway: `agent_ids = []`,
			wantErr: "agent_ids must be a non-empty array",
		},
		{
			name:    "blank plural ID",
			gateway: `agent_ids = ["primary-agent", "   "]`,
			wantErr: "agent_ids[1] must be a non-empty string",
		},
		{
			name: "singular and plural IDs",
			gateway: `agent_id = "primary-agent"
agent_ids = ["enclave-agent"]`,
			wantErr: "gateway.agent_ids cannot be combined",
		},
		{
			name: "legacy and plural IDs",
			gateway: `api_key = "primary-agent"
agent_ids = ["enclave-agent"]`,
			wantErr: "gateway.agent_ids cannot be combined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempTOML(t, "[gateway]\n"+tt.gateway+`

[servers.github]
command = "docker"
args = ["run", "--rm", "-i", "ghcr.io/github/github-mcp-server:latest"]
`)

			cfg, err := LoadFromFile(path)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg.Gateway)
			assert.Equal(t, []string{"primary-agent", "enclave-agent"}, cfg.Gateway.AgentIDs)
		})
	}
}

func TestLoadFromStdin_AgentIDs(t *testing.T) {
	tests := []struct {
		name    string
		agent   string
		wantErr string
	}{
		{
			name:  "valid plural IDs are preserved",
			agent: `"agentIds": ["primary-agent", "enclave-agent"]`,
		},
		{
			name:    "empty plural IDs",
			agent:   `"agentIds": []`,
			wantErr: "agentIds",
		},
		{
			name:    "blank plural ID",
			agent:   `"agentIds": ["primary-agent", " "]`,
			wantErr: "agentIds[1] must be a non-empty string",
		},
		{
			name:    "singular and plural IDs",
			agent:   `"agentId": "primary-agent", "agentIds": ["enclave-agent"]`,
			wantErr: "'oneOf' failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{
				"mcpServers": {"github": {"type": "stdio", "container": "ghcr.io/github/github-mcp-server:latest"}},
				"gateway": {"port": 3000, "domain": "localhost", ` + tt.agent + `}
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
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, []string{"primary-agent", "enclave-agent"}, cfg.Gateway.AgentIDs)
		})
	}
}

func TestValidateGatewayConfig_RejectsLegacyAndPluralIDs(t *testing.T) {
	err := validateGatewayConfig(&StdinGatewayConfig{
		AgentIDs:        []string{"enclave-agent"},
		agentIDsSet:     true,
		legacyAPIKeySet: true,
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "gateway.apiKey cannot be combined with gateway.agentIds")
}
