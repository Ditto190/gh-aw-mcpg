package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEffectiveContainerRuntimeName covers the uncovered branch where
// MCP_GATEWAY_CONTAINER_RUNTIME is set to an invalid value (not docker/podman).
func TestEffectiveContainerRuntimeName_InvalidEnvVar(t *testing.T) {
	t.Run("invalid MCP_GATEWAY_CONTAINER_RUNTIME value is ignored, falls back to config", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "invalidruntime")
		// A non-default config value proves the invalid environment override falls back to config.
		got := effectiveContainerRuntimeName("podman")
		assert.Equal(t, "podman", got, "invalid env var should be ignored, not used")
	})

	t.Run("MCP_GATEWAY_CONTAINER_RUNTIME=podman overrides config docker", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "podman")
		got := effectiveContainerRuntimeName("docker")
		assert.Equal(t, "podman", got)
	})

	t.Run("whitespace-only MCP_GATEWAY_CONTAINER_RUNTIME is ignored", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "   ")
		got := effectiveContainerRuntimeName("podman")
		assert.Equal(t, "podman", got, "whitespace-only env var should be ignored")
	})

	t.Run("empty MCP_GATEWAY_CONTAINER_RUNTIME uses config value", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "")
		got := effectiveContainerRuntimeName("podman")
		assert.Equal(t, "podman", got)
	})
}

// TestConfiguredContainerRuntimeCommand covers the uncovered branch where
// gateway.ContainerRuntimeCommand is non-empty.
func TestConfiguredContainerRuntimeCommand(t *testing.T) {
	tests := []struct {
		name    string
		gateway *GatewayConfig
		want    string
	}{
		{
			name:    "nil gateway returns default docker command",
			gateway: nil,
			want:    "docker",
		},
		{
			name:    "empty ContainerRuntime defaults to docker",
			gateway: &GatewayConfig{ContainerRuntime: ""},
			want:    "docker",
		},
		{
			name:    "podman ContainerRuntime returns podman",
			gateway: &GatewayConfig{ContainerRuntime: "podman"},
			want:    "podman",
		},
		{
			name:    "custom ContainerRuntimeCommand overrides runtime name",
			gateway: &GatewayConfig{ContainerRuntimeCommand: "/usr/local/bin/docker"},
			want:    "/usr/local/bin/docker",
		},
		{
			name:    "whitespace-only ContainerRuntimeCommand is ignored",
			gateway: &GatewayConfig{ContainerRuntimeCommand: "   "},
			want:    "docker",
		},
		{
			name: "ContainerRuntimeCommand takes precedence over ContainerRuntime",
			gateway: &GatewayConfig{
				ContainerRuntime:        "podman",
				ContainerRuntimeCommand: "custom-runtime",
			},
			want: "custom-runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configuredContainerRuntimeCommand(tt.gateway)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestValidateContainerRuntimeCommandNotBlank covers the whitespace-only branch.
func TestValidateContainerRuntimeCommandNotBlank(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty string is valid (field omitted)",
			command: "",
			wantErr: false,
		},
		{
			name:    "non-empty valid command is valid",
			command: "docker",
			wantErr: false,
		},
		{
			name:    "custom path is valid",
			command: "/usr/local/bin/nerdctl",
			wantErr: false,
		},
		{
			name:    "whitespace-only is rejected",
			command: "   ",
			wantErr: true,
			errMsg:  "cannot be empty or whitespace only",
		},
		{
			name:    "tab-only is rejected",
			command: "\t",
			wantErr: true,
			errMsg:  "cannot be empty or whitespace only",
		},
		{
			name:    "newline-only is rejected",
			command: "\n",
			wantErr: true,
			errMsg:  "cannot be empty or whitespace only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerRuntimeCommandNotBlank(tt.command, "containerRuntimeCommand", "gateway.containerRuntimeCommand")
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.ErrorContains(t, err, tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateGatewayPayloadSizeThreshold covers both branches of the standalone helper.
func TestValidateGatewayPayloadSizeThreshold(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{
			name:    "positive value is valid",
			value:   1,
			wantErr: false,
		},
		{
			name:    "large positive value is valid",
			value:   1048576,
			wantErr: false,
		},
		{
			name:    "zero is rejected",
			value:   0,
			wantErr: true,
		},
		{
			name:    "negative value is rejected",
			value:   -1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGatewayPayloadSizeThreshold(tt.value, "payloadSizeThreshold", "gateway.payloadSizeThreshold")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
