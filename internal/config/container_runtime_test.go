package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNonEmptyWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string", "", false},
		{"single space", " ", true},
		{"multiple spaces", "   ", true},
		{"tab", "\t", true},
		{"newline", "\n", true},
		{"mixed whitespace", " \t\n", true},
		{"non-whitespace", "docker", false},
		{"leading whitespace with content", " docker", false},
		{"trailing whitespace with content", "docker ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNonEmptyWhitespace(tt.input))
		})
	}
}

func TestNormalizeContainerRuntime(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"docker lowercase", "docker", "docker"},
		{"docker uppercase", "DOCKER", "docker"},
		{"docker mixed case", "Docker", "docker"},
		{"podman lowercase", "podman", "podman"},
		{"podman uppercase", "PODMAN", "podman"},
		{"with leading space", " docker", "docker"},
		{"with trailing space", "docker ", "docker"},
		{"with surrounding spaces", "  podman  ", "podman"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeContainerRuntime(tt.input))
		})
	}
}

func TestRuntimeCommandForName(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		want    string
	}{
		{"empty defaults to docker", "", "docker"},
		{"docker", "docker", "docker"},
		{"docker uppercase", "DOCKER", "docker"},
		{"podman", "podman", "podman"},
		{"podman uppercase", "PODMAN", "podman"},
		{"podman mixed", "Podman", "podman"},
		{"unknown defaults to docker", "containerd", "docker"},
		{"nerdctl defaults to docker", "nerdctl", "docker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, runtimeCommandForName(tt.runtime))
		})
	}
}

func TestValidateContainerRuntimeValue(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		wantErr   bool
		errSubstr string
	}{
		{"empty string is valid (means default)", "", "gateway.container_runtime", false, ""},
		{"docker is valid", "docker", "gateway.container_runtime", false, ""},
		{"DOCKER is valid (case insensitive)", "DOCKER", "gateway.container_runtime", false, ""},
		{"podman is valid", "podman", "gateway.container_runtime", false, ""},
		{"PODMAN is valid", "PODMAN", "gateway.container_runtime", false, ""},
		{"whitespace only is invalid", "   ", "gateway.container_runtime", true, "must not be empty or whitespace-only"},
		{"tab only is invalid", "\t", "gateway.container_runtime", true, "must not be empty or whitespace-only"},
		{"unknown runtime is invalid", "containerd", "gateway.container_runtime", true, "must be one of: docker, podman"},
		{"nerdctl is invalid", "nerdctl", "gateway.container_runtime", true, "must be one of: docker, podman"},
		{"error includes field name", "invalid", "my.field", true, "my.field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerRuntimeValue(tt.value, tt.fieldName)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfiguredContainerRuntimeName(t *testing.T) {
	tests := []struct {
		name          string
		configRuntime string
		want          string
	}{
		{"empty uses default (docker)", "", DefaultContainerRuntime},
		{"docker", "docker", "docker"},
		{"Docker normalized", "Docker", "docker"},
		{"DOCKER normalized", "DOCKER", "docker"},
		{"podman", "podman", "podman"},
		{"PODMAN normalized", "PODMAN", "podman"},
		{"whitespace-only uses default", "   ", DefaultContainerRuntime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, configuredContainerRuntimeName(tt.configRuntime))
		})
	}
}

func TestEffectiveContainerRuntimeName(t *testing.T) {
	const envKey = "MCP_GATEWAY_CONTAINER_RUNTIME"

	restoreEnv := func(old string, wasSet bool) {
		if wasSet {
			os.Setenv(envKey, old)
		} else {
			os.Unsetenv(envKey)
		}
	}

	tests := []struct {
		name          string
		configRuntime string
		envValue      string
		envSet        bool
		want          string
	}{
		{
			name:          "no config no env uses default",
			configRuntime: "",
			envSet:        false,
			want:          DefaultContainerRuntime,
		},
		{
			name:          "config docker no env override",
			configRuntime: "docker",
			envSet:        false,
			want:          "docker",
		},
		{
			name:          "config podman no env override",
			configRuntime: "podman",
			envSet:        false,
			want:          "podman",
		},
		{
			name:          "env overrides config docker→podman",
			configRuntime: "docker",
			envValue:      "podman",
			envSet:        true,
			want:          "podman",
		},
		{
			name:          "env overrides config podman→docker",
			configRuntime: "podman",
			envValue:      "docker",
			envSet:        true,
			want:          "docker",
		},
		{
			name:          "whitespace-only env is ignored, uses config",
			configRuntime: "podman",
			envValue:      "   ",
			envSet:        true,
			want:          "podman",
		},
		{
			name:          "invalid env is ignored, uses config",
			configRuntime: "docker",
			envValue:      "containerd",
			envSet:        true,
			want:          "docker",
		},
		{
			name:          "invalid env is ignored, falls back to default",
			configRuntime: "",
			envValue:      "nerdctl",
			envSet:        true,
			want:          DefaultContainerRuntime,
		},
		{
			name:          "env uppercase PODMAN is normalized",
			configRuntime: "docker",
			envValue:      "PODMAN",
			envSet:        true,
			want:          "podman",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old, wasSet := os.LookupEnv(envKey)
			defer restoreEnv(old, wasSet)

			if tt.envSet {
				os.Setenv(envKey, tt.envValue)
			} else {
				os.Unsetenv(envKey)
			}

			got := effectiveContainerRuntimeName(tt.configRuntime)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEffectiveContainerRuntimeCommand(t *testing.T) {
	const envKey = "MCP_GATEWAY_CONTAINER_RUNTIME"

	restoreEnv := func(old string, wasSet bool) {
		if wasSet {
			os.Setenv(envKey, old)
		} else {
			os.Unsetenv(envKey)
		}
	}

	old, wasSet := os.LookupEnv(envKey)
	defer restoreEnv(old, wasSet)
	os.Unsetenv(envKey)

	tests := []struct {
		name    string
		gateway *GatewayConfig
		want    string
	}{
		{
			name:    "nil gateway uses default docker command",
			gateway: nil,
			want:    "docker",
		},
		{
			name:    "empty gateway uses docker",
			gateway: &GatewayConfig{},
			want:    "docker",
		},
		{
			name:    "gateway with docker runtime",
			gateway: &GatewayConfig{ContainerRuntime: "docker"},
			want:    "docker",
		},
		{
			name:    "gateway with podman runtime",
			gateway: &GatewayConfig{ContainerRuntime: "podman"},
			want:    "podman",
		},
		{
			name:    "gateway with custom command overrides runtime",
			gateway: &GatewayConfig{ContainerRuntime: "podman", ContainerRuntimeCommand: "/usr/bin/podman"},
			want:    "/usr/bin/podman",
		},
		{
			name:    "gateway with whitespace command uses runtime",
			gateway: &GatewayConfig{ContainerRuntime: "podman", ContainerRuntimeCommand: "   "},
			want:    "podman",
		},
		{
			name:    "custom command on docker runtime",
			gateway: &GatewayConfig{ContainerRuntime: "docker", ContainerRuntimeCommand: "nerdctl"},
			want:    "nerdctl",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, effectiveContainerRuntimeCommand(tt.gateway))
		})
	}
}

func TestEffectiveContainerRuntimeCommand_EnvOverride(t *testing.T) {
	const envKey = "MCP_GATEWAY_CONTAINER_RUNTIME"

	old, wasSet := os.LookupEnv(envKey)
	defer func() {
		if wasSet {
			os.Setenv(envKey, old)
		} else {
			os.Unsetenv(envKey)
		}
	}()

	// Env overrides config runtime but not ContainerRuntimeCommand
	os.Setenv(envKey, "podman")
	gw := &GatewayConfig{ContainerRuntime: "docker"}
	assert.Equal(t, "podman", effectiveContainerRuntimeCommand(gw))

	// Custom command still takes precedence over env override
	gw2 := &GatewayConfig{ContainerRuntime: "docker", ContainerRuntimeCommand: "/custom/docker"}
	assert.Equal(t, "/custom/docker", effectiveContainerRuntimeCommand(gw2))
}

func TestConfiguredContainerRuntimeCommand(t *testing.T) {
	tests := []struct {
		name    string
		gateway *GatewayConfig
		want    string
	}{
		{
			name:    "nil gateway uses default",
			gateway: nil,
			want:    "docker",
		},
		{
			name:    "empty gateway uses docker",
			gateway: &GatewayConfig{},
			want:    "docker",
		},
		{
			name:    "docker runtime",
			gateway: &GatewayConfig{ContainerRuntime: "docker"},
			want:    "docker",
		},
		{
			name:    "podman runtime",
			gateway: &GatewayConfig{ContainerRuntime: "podman"},
			want:    "podman",
		},
		{
			name:    "custom command overrides",
			gateway: &GatewayConfig{ContainerRuntime: "podman", ContainerRuntimeCommand: "/opt/podman"},
			want:    "/opt/podman",
		},
		{
			name:    "whitespace command ignored",
			gateway: &GatewayConfig{ContainerRuntime: "podman", ContainerRuntimeCommand: "  "},
			want:    "podman",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, configuredContainerRuntimeCommand(tt.gateway))
		})
	}
}
