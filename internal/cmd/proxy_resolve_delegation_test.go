package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/delegation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validDelegationEnvelopeJSON returns a minimal, valid delegation.EnvelopeWire
// serialized as JSON for use in resolveDelegationProxyConfig tests.
func validDelegationEnvelopeJSON(t *testing.T) string {
	t.Helper()
	envelope := delegation.EnvelopeWire{
		RunID:                  "run-1",
		EnclaveBackend:         "backend-1",
		AllowedRepositories:    []string{"owner/repo"},
		ToolPolicy:             delegation.ToolPolicyGitHubRepositoryReadV1,
		MaxDynamicSchemaHashes: 1,
		MaxIdentityTTLSeconds:  300,
		ExpiresAt:              time.Now().Add(time.Hour),
	}
	raw, err := json.Marshal(envelope)
	require.NoError(t, err)
	return string(raw)
}

func setAllDelegationEnvVars(t *testing.T, envelopeJSON, capabilityKey, statePath, generation, controlListenAddr string) {
	t.Helper()
	t.Setenv("MCP_GATEWAY_DELEGATION_ENVELOPE", envelopeJSON)
	t.Setenv(delegation.EnvControlCapabilityKey, capabilityKey)
	t.Setenv("MCP_GATEWAY_DELEGATION_STATE_PATH", statePath)
	t.Setenv("MCP_GATEWAY_DELEGATION_GENERATION", generation)
	t.Setenv(delegation.EnvControlListenAddr, controlListenAddr)
}

func TestResolveDelegationProxyConfig_AllEnvVarsUnset_ReturnsNilNilNil(t *testing.T) {
	t.Setenv("MCP_GATEWAY_DELEGATION_ENVELOPE", "")
	t.Setenv(delegation.EnvControlCapabilityKey, "")
	t.Setenv("MCP_GATEWAY_DELEGATION_STATE_PATH", "")
	t.Setenv("MCP_GATEWAY_DELEGATION_GENERATION", "")
	t.Setenv(delegation.EnvControlListenAddr, "")

	cfg, statePath, err := resolveDelegationProxyConfig()

	require.NoError(t, err)
	assert.Nil(t, cfg)
	assert.Empty(t, statePath)
}

func TestResolveDelegationProxyConfig_PartiallySet_ReturnsError(t *testing.T) {
	tests := []struct {
		name              string
		envelopeJSON      string
		capabilityKey     string
		statePath         string
		generation        string
		controlListenAddr string
	}{
		{
			name:              "only envelope set",
			envelopeJSON:      "{}",
			capabilityKey:     "",
			statePath:         "",
			generation:        "",
			controlListenAddr: "",
		},
		{
			name:              "only capability key set",
			envelopeJSON:      "",
			capabilityKey:     "some-capability-key-that-is-long-enough",
			statePath:         "",
			generation:        "",
			controlListenAddr: "",
		},
		{
			name:              "only state path set",
			envelopeJSON:      "",
			capabilityKey:     "",
			statePath:         "/tmp/gh-aw/agent/state.json",
			generation:        "",
			controlListenAddr: "",
		},
		{
			name:              "only generation set",
			envelopeJSON:      "",
			capabilityKey:     "",
			statePath:         "",
			generation:        "1",
			controlListenAddr: "",
		},
		{
			name:              "only control listen addr set",
			envelopeJSON:      "",
			capabilityKey:     "",
			statePath:         "",
			generation:        "",
			controlListenAddr: "127.0.0.1:9999",
		},
		{
			name:              "four of five set, missing state path",
			envelopeJSON:      "{}",
			capabilityKey:     "some-capability-key-that-is-long-enough",
			statePath:         "",
			generation:        "1",
			controlListenAddr: "127.0.0.1:9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setAllDelegationEnvVars(t, tt.envelopeJSON, tt.capabilityKey, tt.statePath, tt.generation, tt.controlListenAddr)

			cfg, statePath, err := resolveDelegationProxyConfig()

			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Empty(t, statePath)
			assert.Contains(t, err.Error(), "must be configured together")
		})
	}
}

func TestResolveDelegationProxyConfig_InvalidEnvelopeJSON_ReturnsError(t *testing.T) {
	setAllDelegationEnvVars(t, "not-json", "some-capability-key-that-is-long-enough", "/tmp/gh-aw/agent/state.json", "1", "127.0.0.1:9999")

	cfg, statePath, err := resolveDelegationProxyConfig()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Empty(t, statePath)
	assert.Contains(t, err.Error(), "invalid delegation envelope")
}

func TestResolveDelegationProxyConfig_TrailingJSON_ReturnsError(t *testing.T) {
	envelopeJSON := validDelegationEnvelopeJSON(t) + `{"extra":true}`
	setAllDelegationEnvVars(t, envelopeJSON, "some-capability-key-that-is-long-enough", "/tmp/gh-aw/agent/state.json", "1", "127.0.0.1:9999")

	cfg, statePath, err := resolveDelegationProxyConfig()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Empty(t, statePath)
	assert.Contains(t, err.Error(), "trailing JSON")
}

func TestResolveDelegationProxyConfig_UnknownFieldInEnvelope_ReturnsError(t *testing.T) {
	envelopeJSON := `{"run_id":"run-1","enclave_backend":"backend-1","allowed_repositories":["owner/repo"],"tool_policy":"github-repository-read-v1","max_dynamic_schema_hashes":1,"max_identity_ttl":300000000000,"expires_at":"2099-01-01T00:00:00Z","unknown_field":true}`
	setAllDelegationEnvVars(t, envelopeJSON, "some-capability-key-that-is-long-enough", "/tmp/gh-aw/agent/state.json", "1", "127.0.0.1:9999")

	cfg, statePath, err := resolveDelegationProxyConfig()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Empty(t, statePath)
	assert.Contains(t, err.Error(), "invalid delegation envelope")
}

func TestResolveDelegationProxyConfig_InvalidGeneration_ReturnsError(t *testing.T) {
	envelopeJSON := validDelegationEnvelopeJSON(t)
	setAllDelegationEnvVars(t, envelopeJSON, "some-capability-key-that-is-long-enough", "/tmp/gh-aw/agent/state.json", "not-a-number", "127.0.0.1:9999")

	cfg, statePath, err := resolveDelegationProxyConfig()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Empty(t, statePath)
	assert.Contains(t, err.Error(), "invalid delegation generation")
}

func TestResolveDelegationProxyConfig_InvalidCapabilityKey_ReturnsError(t *testing.T) {
	envelopeJSON := validDelegationEnvelopeJSON(t)
	// Capability key shorter than the required 32-byte minimum.
	setAllDelegationEnvVars(t, envelopeJSON, "too-short", "/tmp/gh-aw/agent/state.json", "1", "127.0.0.1:9999")

	cfg, statePath, err := resolveDelegationProxyConfig()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Empty(t, statePath)
	assert.Contains(t, err.Error(), "delegation control capability")
}

func TestResolveDelegationProxyConfig_InvalidEnvelopeInvariant_LoadStoreFails(t *testing.T) {
	// Envelope JSON decodes fine but fails Envelope.Validate() invariants
	// (missing run_id), which LoadStore -> NewStore should reject.
	envelopeJSON := `{"run_id":"","enclave_backend":"backend-1","allowed_repositories":["owner/repo"],"tool_policy":"github-repository-read-v1","max_dynamic_schema_hashes":1,"max_identity_ttl":300000000000,"expires_at":"2099-01-01T00:00:00Z"}`
	statePath := filepath.Join(t.TempDir(), "state.json")
	setAllDelegationEnvVars(t, envelopeJSON, "some-capability-key-that-is-long-enough", statePath, "1", "127.0.0.1:9999")

	cfg, gotStatePath, err := resolveDelegationProxyConfig()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Empty(t, gotStatePath)
}

func TestResolveDelegationProxyConfig_Success_ReturnsPopulatedConfig(t *testing.T) {
	envelopeJSON := validDelegationEnvelopeJSON(t)
	statePath := filepath.Join(t.TempDir(), "state.json")
	capabilityKey := "some-capability-key-that-is-long-enough"
	controlListenAddr := "127.0.0.1:9999"
	setAllDelegationEnvVars(t, envelopeJSON, capabilityKey, statePath, "1", controlListenAddr)

	cfg, gotStatePath, err := resolveDelegationProxyConfig()

	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.NotNil(t, cfg.Store)
	assert.NotNil(t, cfg.Capability)
	assert.Equal(t, statePath, cfg.StatePath)
	assert.Equal(t, controlListenAddr, cfg.ControlListenAddr)
	assert.Equal(t, statePath, gotStatePath)
}

func TestResolveDelegationProxyConfig_NoPriorStateFile_StartsFresh(t *testing.T) {
	envelopeJSON := validDelegationEnvelopeJSON(t)
	// Nonexistent state file path: LoadStore should treat as "no prior state"
	// and succeed with a fresh, empty store.
	statePath := filepath.Join(t.TempDir(), "state.json")
	setAllDelegationEnvVars(t, envelopeJSON, "some-capability-key-that-is-long-enough", statePath, "42", "127.0.0.1:9999")

	cfg, gotStatePath, err := resolveDelegationProxyConfig()

	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, statePath, gotStatePath)
}
