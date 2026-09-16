// Package config tests: coverage for the agent-selection branches of
// validateGatewayConfigWithAgentRequirement (internal/config/validation_gateway.go)
// that were not exercised by any existing test:
//   - gateway.agentId explicitly set to an empty/whitespace string (agentIDSet
//     true, TrimSpace(AgentID) == "")
//   - gateway.apiKey explicitly set to an empty/whitespace string
//     (legacyAPIKeySet true, TrimSpace(APIKey) == "")
//   - the exactly-one-agent-selection requirement rejecting a gateway with no
//     agentId/agentIds/apiKey set at all, via the public validateGatewayConfig
//     entry point (requireAgentSelection == true)
//   - gateway.agentId combined with gateway.agentIds rejected
//   - validateContainerRuntimeCommandNotBlank's error propagated through
//     validateGatewayConfigWithAgentRequirement for a whitespace-only
//     containerRuntimeCommand
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateGatewayConfig_AgentIDSetButEmpty covers the branch where
// gateway.agentId is present in the source document (agentIDSet == true) but
// its trimmed value is empty, which must be rejected regardless of whether
// agent selection is otherwise required.
func TestValidateGatewayConfig_AgentIDSetButEmpty(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
	}{
		{name: "empty string", agentID: ""},
		{name: "whitespace only", agentID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &StdinGatewayConfig{
				AgentID:    tt.agentID,
				agentIDSet: true,
			}

			err := validateGatewayConfig(gateway)
			require.Error(t, err)
			assert.ErrorContains(t, err, "gateway.agentId must be a non-empty string when provided")
		})
	}
}

// TestValidateGatewayConfig_LegacyAPIKeySetButEmpty covers the branch where
// the deprecated gateway.apiKey alias is present in the source document
// (legacyAPIKeySet == true) but its trimmed value is empty.
func TestValidateGatewayConfig_LegacyAPIKeySetButEmpty(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
	}{
		{name: "empty string", apiKey: ""},
		{name: "whitespace only", apiKey: "\t\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &StdinGatewayConfig{
				APIKey:          tt.apiKey,
				legacyAPIKeySet: true,
			}

			err := validateGatewayConfig(gateway)
			require.Error(t, err)
			assert.ErrorContains(t, err, "gateway.apiKey must be a non-empty string when provided")
		})
	}
}

// TestValidateGatewayConfig_RequiresExactlyOneAgentSelection covers the
// requireAgentSelection branch: when validateGatewayConfig (which always
// requires a selection) is called with a gateway that has no agentId,
// agentIds, or apiKey configured at all, it must reject the config.
func TestValidateGatewayConfig_RequiresExactlyOneAgentSelection(t *testing.T) {
	err := validateGatewayConfig(&StdinGatewayConfig{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "gateway.agentId or gateway.agentIds must be configured; exactly one selection is required")
}

// TestValidateGatewayConfigForPatterns_AllowsMissingAgentSelection verifies
// the counterpart entry point (requireAgentSelection == false) does NOT
// reject a gateway with no agent selection at all, confirming the
// requireAgentSelection flag actually gates the check exercised above.
func TestValidateGatewayConfigForPatterns_AllowsMissingAgentSelection(t *testing.T) {
	err := validateGatewayConfigForPatterns(&StdinGatewayConfig{})
	assert.NoError(t, err)
}

// TestValidateGatewayConfig_RejectsAgentIDCombinedWithAgentIDs covers the
// branch rejecting gateway.agentId combined with gateway.agentIds, mirroring
// the existing legacy-apiKey-plus-agentIds test but for the primary
// agentId field.
func TestValidateGatewayConfig_RejectsAgentIDCombinedWithAgentIDs(t *testing.T) {
	err := validateGatewayConfig(&StdinGatewayConfig{
		AgentID:     "primary-agent",
		AgentIDs:    []string{"enclave-agent"},
		agentIDSet:  true,
		agentIDsSet: true,
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "gateway.agentId cannot be combined with gateway.agentIds")
}

// TestValidateGatewayConfig_ContainerRuntimeCommandWhitespaceOnly covers the
// validateContainerRuntimeCommandNotBlank error path as invoked from
// validateGatewayConfigWithAgentRequirement (as opposed to the direct unit
// tests of validateContainerRuntimeCommandNotBlank in
// internal/config/container_runtime_test.go).
func TestValidateGatewayConfig_ContainerRuntimeCommandWhitespaceOnly(t *testing.T) {
	err := validateGatewayConfig(&StdinGatewayConfig{
		AgentID:                 "test-agent",
		ContainerRuntimeCommand: "   ",
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "containerRuntimeCommand cannot be empty or whitespace only")
}
