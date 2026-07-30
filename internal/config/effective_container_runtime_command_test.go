package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEffectiveContainerRuntimeCommand covers all branches of the
// effectiveContainerRuntimeCommand function, which combines:
//   - gateway.ContainerRuntime (config-level runtime preference)
//   - MCP_GATEWAY_CONTAINER_RUNTIME env var (runtime override)
//   - gateway.ContainerRuntimeCommand (explicit command path override)
func TestEffectiveContainerRuntimeCommand(t *testing.T) {
	tests := []struct {
		name    string
		gateway *GatewayConfig
		env     string // MCP_GATEWAY_CONTAINER_RUNTIME value ("" means unset)
		want    string
	}{
		// --- nil gateway ---
		{
			name:    "nil gateway returns docker (default)",
			gateway: nil,
			want:    "docker",
		},
		{
			name:    "nil gateway with env=podman returns podman command",
			gateway: nil,
			env:     "podman",
			want:    "podman",
		},
		{
			name:    "nil gateway with invalid env falls back to default docker",
			gateway: nil,
			env:     "nerdctl",
			want:    "docker",
		},
		{
			name:    "nil gateway with whitespace-only env falls back to default docker",
			gateway: nil,
			env:     "   ",
			want:    "docker",
		},

		// --- empty ContainerRuntime (defaults to docker) ---
		{
			name:    "empty ContainerRuntime defaults to docker",
			gateway: &GatewayConfig{ContainerRuntime: ""},
			want:    "docker",
		},
		{
			name:    "empty ContainerRuntime with env=podman uses podman",
			gateway: &GatewayConfig{ContainerRuntime: ""},
			env:     "podman",
			want:    "podman",
		},

		// --- ContainerRuntime=docker ---
		{
			name:    "docker ContainerRuntime returns docker command",
			gateway: &GatewayConfig{ContainerRuntime: "docker"},
			want:    "docker",
		},
		{
			name:    "docker ContainerRuntime with env=podman overrides to podman",
			gateway: &GatewayConfig{ContainerRuntime: "docker"},
			env:     "podman",
			want:    "podman",
		},

		// --- ContainerRuntime=podman ---
		{
			name:    "podman ContainerRuntime returns podman command",
			gateway: &GatewayConfig{ContainerRuntime: "podman"},
			want:    "podman",
		},
		{
			name:    "podman ContainerRuntime with env=docker overrides to docker",
			gateway: &GatewayConfig{ContainerRuntime: "podman"},
			env:     "docker",
			want:    "docker",
		},

		// --- ContainerRuntime case-insensitivity ---
		{
			name:    "ContainerRuntime=PODMAN (uppercase) is normalised to podman",
			gateway: &GatewayConfig{ContainerRuntime: "PODMAN"},
			want:    "podman",
		},
		{
			name:    "ContainerRuntime=Docker (mixed case) is normalised to docker",
			gateway: &GatewayConfig{ContainerRuntime: "Docker"},
			want:    "docker",
		},
		{
			name:    "ContainerRuntime with surrounding whitespace is trimmed",
			gateway: &GatewayConfig{ContainerRuntime: "  podman  "},
			want:    "podman",
		},

		// --- ContainerRuntimeCommand override ---
		{
			name:    "ContainerRuntimeCommand overrides default runtime command",
			gateway: &GatewayConfig{ContainerRuntimeCommand: "/usr/local/bin/docker"},
			want:    "/usr/local/bin/docker",
		},
		{
			name:    "ContainerRuntimeCommand overrides podman runtime",
			gateway: &GatewayConfig{ContainerRuntime: "podman", ContainerRuntimeCommand: "/opt/bin/podman"},
			want:    "/opt/bin/podman",
		},
		{
			name:    "ContainerRuntimeCommand overrides env var",
			gateway: &GatewayConfig{ContainerRuntimeCommand: "custom-runtime"},
			env:     "podman",
			want:    "custom-runtime",
		},
		{
			name:    "whitespace-only ContainerRuntimeCommand is ignored; falls back to runtime",
			gateway: &GatewayConfig{ContainerRuntimeCommand: "   "},
			want:    "docker",
		},
		{
			name:    "whitespace-only ContainerRuntimeCommand with podman runtime returns podman",
			gateway: &GatewayConfig{ContainerRuntime: "podman", ContainerRuntimeCommand: "\t"},
			want:    "podman",
		},
		{
			name:    "ContainerRuntimeCommand leading/trailing whitespace is trimmed",
			gateway: &GatewayConfig{ContainerRuntimeCommand: "  /bin/docker  "},
			want:    "/bin/docker",
		},

		// --- env override with ContainerRuntimeCommand ---
		{
			name:    "env=podman and ContainerRuntimeCommand set; command wins",
			gateway: &GatewayConfig{ContainerRuntime: "docker", ContainerRuntimeCommand: "my-docker"},
			env:     "podman",
			want:    "my-docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", tt.env)
			} else {
				// Ensure env is not set from parent test environment.
				t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "")
			}
			got := effectiveContainerRuntimeCommand(tt.gateway)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIsNonEmptyWhitespace covers all branches of the isNonEmptyWhitespace helper.
func TestIsNonEmptyWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty string is not whitespace-only", "", false},
		{"single space is whitespace-only", " ", true},
		{"tab is whitespace-only", "\t", true},
		{"newline is whitespace-only", "\n", true},
		{"mixed whitespace is whitespace-only", " \t\n ", true},
		{"non-whitespace string is false", "docker", false},
		{"string with leading space but content is false", " docker", false},
		{"string with trailing space but content is false", "docker ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNonEmptyWhitespace(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNormalizeContainerRuntime covers the normalizeContainerRuntime helper.
func TestNormalizeContainerRuntime(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string returns empty", "", ""},
		{"docker unchanged", "docker", "docker"},
		{"podman unchanged", "podman", "podman"},
		{"DOCKER uppercased is lowercased", "DOCKER", "docker"},
		{"PODMAN uppercased is lowercased", "PODMAN", "podman"},
		{"leading whitespace is trimmed", "  docker", "docker"},
		{"trailing whitespace is trimmed", "docker  ", "docker"},
		{"surrounding whitespace is trimmed", "  podman  ", "podman"},
		{"mixed case with whitespace", "  Docker  ", "docker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeContainerRuntime(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestEffectiveContainerRuntimeCommand_PublicWrapper verifies that the exported
// EffectiveContainerRuntimeCommand delegates to the internal implementation and
// returns the expected runtime command for representative inputs. This test
// specifically exercises the exported wrapper (line 112 of container_runtime.go)
// which is distinct from the internal effectiveContainerRuntimeCommand tests above.
func TestEffectiveContainerRuntimeCommand_PublicWrapper(t *testing.T) {
	t.Run("nil gateway returns docker", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "")
		assert.Equal(t, "docker", EffectiveContainerRuntimeCommand(nil))
	})

	t.Run("podman ContainerRuntime returns podman", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "")
		assert.Equal(t, "podman", EffectiveContainerRuntimeCommand(&GatewayConfig{ContainerRuntime: "podman"}))
	})

	t.Run("env override takes effect", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "podman")
		assert.Equal(t, "podman", EffectiveContainerRuntimeCommand(nil))
	})

	t.Run("ContainerRuntimeCommand overrides env", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "podman")
		assert.Equal(t, "/usr/bin/docker", EffectiveContainerRuntimeCommand(&GatewayConfig{
			ContainerRuntimeCommand: "/usr/bin/docker",
		}))
	})
}

// TestRuntimeCommandForName covers the runtimeCommandForName helper.
func TestRuntimeCommandForName(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		want    string
	}{
		{"docker returns docker", "docker", "docker"},
		{"podman returns podman", "podman", "podman"},
		{"empty returns docker (default)", "", "docker"},
		{"unknown runtime returns docker (fallback)", "nerdctl", "docker"},
		{"PODMAN uppercase returns podman (normalization)", "PODMAN", "podman"},
		{"DOCKER uppercase returns docker (normalization)", "DOCKER", "docker"},
		{"podman with whitespace returns podman (normalization)", "  podman  ", "podman"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runtimeCommandForName(tt.runtime)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestEffectiveContainerRuntimeCommand_PublicWrapper verifies that the exported
// EffectiveContainerRuntimeCommand function delegates to the private implementation
// and returns the same result.
func TestEffectiveContainerRuntimeCommand_PublicWrapper(t *testing.T) {
	tests := []struct {
		name    string
		gateway *GatewayConfig
		want    string
	}{
		{
			name:    "nil gateway returns docker default",
			gateway: nil,
			want:    "docker",
		},
		{
			name: "explicit docker runtime",
			gateway: &GatewayConfig{
				ContainerRuntime: "docker",
			},
			want: "docker",
		},
		{
			name: "explicit podman runtime",
			gateway: &GatewayConfig{
				ContainerRuntime: "podman",
			},
			want: "podman",
		},
		{
			name: "explicit command path override",
			gateway: &GatewayConfig{
				ContainerRuntimeCommand: "/usr/local/bin/docker",
			},
			want: "/usr/local/bin/docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always clear the env var so the test is hermetic regardless of the
			// parent environment (e.g. MCP_GATEWAY_CONTAINER_RUNTIME=podman).
			t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "")
			got := EffectiveContainerRuntimeCommand(tt.gateway)
			assert.Equal(t, tt.want, got,
				"EffectiveContainerRuntimeCommand should delegate to private implementation")
			// Also verify agreement with the private function
			assert.Equal(t, effectiveContainerRuntimeCommand(tt.gateway), got,
				"public and private implementations must agree")
		})
	}
}
