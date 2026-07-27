package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNonEmptyWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty string returns false", input: "", want: false},
		{name: "single space returns true", input: " ", want: true},
		{name: "multiple spaces returns true", input: "   ", want: true},
		{name: "tab returns true", input: "\t", want: true},
		{name: "newline returns true", input: "\n", want: true},
		{name: "mixed whitespace returns true", input: " \t\n ", want: true},
		{name: "non-whitespace returns false", input: "docker", want: false},
		{name: "mixed string with content returns false", input: " docker ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNonEmptyWhitespace(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeContainerRuntime(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string", input: "", want: ""},
		{name: "docker lowercase", input: "docker", want: "docker"},
		{name: "docker uppercase", input: "DOCKER", want: "docker"},
		{name: "docker mixed case", input: "Docker", want: "docker"},
		{name: "podman lowercase", input: "podman", want: "podman"},
		{name: "podman uppercase", input: "PODMAN", want: "podman"},
		{name: "leading trailing spaces", input: "  docker  ", want: "docker"},
		{name: "tabs trimmed", input: "\tdocker\t", want: "docker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeContainerRuntime(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRuntimeCommandForName(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		want    string
	}{
		{name: "docker returns docker", runtime: "docker", want: "docker"},
		{name: "podman returns podman", runtime: "podman", want: "podman"},
		{name: "empty returns docker", runtime: "", want: "docker"},
		{name: "unknown returns docker", runtime: "containerd", want: "docker"},
		{name: "PODMAN uppercase returns podman", runtime: "PODMAN", want: "podman"},
		{name: "DOCKER uppercase returns docker", runtime: "DOCKER", want: "docker"},
		{name: "podman with spaces returns podman", runtime: "  podman  ", want: "podman"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runtimeCommandForName(tt.runtime)
			assert.Equal(t, tt.want, got)
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
		{
			name:      "empty string is valid (means unset)",
			value:     "",
			fieldName: "gateway.containerRuntime",
			wantErr:   false,
		},
		{
			name:      "docker is valid",
			value:     "docker",
			fieldName: "gateway.containerRuntime",
			wantErr:   false,
		},
		{
			name:      "podman is valid",
			value:     "podman",
			fieldName: "gateway.containerRuntime",
			wantErr:   false,
		},
		{
			name:      "DOCKER uppercase is valid",
			value:     "DOCKER",
			fieldName: "gateway.containerRuntime",
			wantErr:   false,
		},
		{
			name:      "PODMAN uppercase is valid",
			value:     "PODMAN",
			fieldName: "gateway.containerRuntime",
			wantErr:   false,
		},
		{
			name:      "whitespace-only string is invalid",
			value:     "   ",
			fieldName: "gateway.containerRuntime",
			wantErr:   true,
			errSubstr: "must not be empty or whitespace-only when set",
		},
		{
			name:      "single space is invalid",
			value:     " ",
			fieldName: "gateway.containerRuntime",
			wantErr:   true,
			errSubstr: "must not be empty or whitespace-only when set",
		},
		{
			name:      "unknown runtime is invalid",
			value:     "containerd",
			fieldName: "gateway.containerRuntime",
			wantErr:   true,
			errSubstr: "must be one of: docker, podman",
		},
		{
			name:      "unknown runtime includes got value in error",
			value:     "nerdctl",
			fieldName: "gateway.containerRuntime",
			wantErr:   true,
			errSubstr: `"nerdctl"`,
		},
		{
			name:      "field name is included in whitespace error",
			value:     " ",
			fieldName: "custom.field",
			wantErr:   true,
			errSubstr: "custom.field",
		},
		{
			name:      "field name is included in unknown value error",
			value:     "invalid",
			fieldName: "custom.field",
			wantErr:   true,
			errSubstr: "custom.field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerRuntimeValue(tt.value, tt.fieldName)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errSubstr != "" {
					assert.ErrorContains(t, err, tt.errSubstr)
				}
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
		{
			name:          "empty config uses default (docker)",
			configRuntime: "",
			want:          "docker",
		},
		{
			name:          "docker config returns docker",
			configRuntime: "docker",
			want:          "docker",
		},
		{
			name:          "podman config returns podman",
			configRuntime: "podman",
			want:          "podman",
		},
		{
			name:          "PODMAN uppercase normalized to podman",
			configRuntime: "PODMAN",
			want:          "podman",
		},
		{
			name:          "spaces around value trimmed",
			configRuntime: "  docker  ",
			want:          "docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configuredContainerRuntimeName(tt.configRuntime)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEffectiveContainerRuntimeName(t *testing.T) {
	tests := []struct {
		name          string
		configRuntime string
		envRuntime    string
		want          string
	}{
		{
			name:          "empty config, no env uses default (docker)",
			configRuntime: "",
			envRuntime:    "",
			want:          "docker",
		},
		{
			name:          "config docker, no env returns docker",
			configRuntime: "docker",
			envRuntime:    "",
			want:          "docker",
		},
		{
			name:          "config podman, no env returns podman",
			configRuntime: "podman",
			envRuntime:    "",
			want:          "podman",
		},
		{
			name:          "env overrides config: env=podman, config=docker",
			configRuntime: "docker",
			envRuntime:    "podman",
			want:          "podman",
		},
		{
			name:          "env overrides config: env=docker, config=podman",
			configRuntime: "podman",
			envRuntime:    "docker",
			want:          "docker",
		},
		{
			name:          "whitespace-only env is ignored, falls back to config",
			configRuntime: "podman",
			envRuntime:    "   ",
			want:          "podman",
		},
		{
			name:          "invalid env value is ignored, falls back to config",
			configRuntime: "docker",
			envRuntime:    "containerd",
			want:          "docker",
		},
		{
			name:          "env PODMAN uppercase is normalized",
			configRuntime: "docker",
			envRuntime:    "PODMAN",
			want:          "podman",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envRuntime != "" {
				t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", tt.envRuntime)
			} else {
				t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "")
			}

			got := effectiveContainerRuntimeName(tt.configRuntime)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEffectiveContainerRuntimeCommand(t *testing.T) {
	tests := []struct {
		name               string
		gateway            *GatewayConfig
		envRuntime         string
		want               string
	}{
		{
			name:    "nil gateway returns default docker command",
			gateway: nil,
			want:    "docker",
		},
		{
			name:    "empty gateway returns docker command",
			gateway: &GatewayConfig{},
			want:    "docker",
		},
		{
			name: "gateway with podman runtime returns podman command",
			gateway: &GatewayConfig{
				ContainerRuntime: "podman",
			},
			want: "podman",
		},
		{
			name: "custom ContainerRuntimeCommand overrides all",
			gateway: &GatewayConfig{
				ContainerRuntime:        "docker",
				ContainerRuntimeCommand: "nerdctl",
			},
			want: "nerdctl",
		},
		{
			name: "whitespace-only ContainerRuntimeCommand is ignored",
			gateway: &GatewayConfig{
				ContainerRuntime:        "podman",
				ContainerRuntimeCommand: "   ",
			},
			want: "podman",
		},
		{
			name: "env runtime overrides config runtime but custom command takes final precedence",
			gateway: &GatewayConfig{
				ContainerRuntime:        "docker",
				ContainerRuntimeCommand: "myruntime",
			},
			envRuntime: "podman",
			want:       "myruntime",
		},
		{
			name: "env runtime overrides config runtime when no custom command",
			gateway: &GatewayConfig{
				ContainerRuntime: "docker",
			},
			envRuntime: "podman",
			want:       "podman",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envRuntime != "" {
				t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", tt.envRuntime)
			} else {
				t.Setenv("MCP_GATEWAY_CONTAINER_RUNTIME", "")
			}

			got := effectiveContainerRuntimeCommand(tt.gateway)
			assert.Equal(t, tt.want, got)
		})
	}
}

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
			name:    "empty gateway returns docker command",
			gateway: &GatewayConfig{},
			want:    "docker",
		},
		{
			name: "gateway with podman runtime returns podman command",
			gateway: &GatewayConfig{
				ContainerRuntime: "podman",
			},
			want: "podman",
		},
		{
			name: "gateway with docker runtime returns docker command",
			gateway: &GatewayConfig{
				ContainerRuntime: "docker",
			},
			want: "docker",
		},
		{
			name: "custom ContainerRuntimeCommand takes precedence over runtime name",
			gateway: &GatewayConfig{
				ContainerRuntime:        "podman",
				ContainerRuntimeCommand: "nerdctl",
			},
			want: "nerdctl",
		},
		{
			name: "whitespace-only ContainerRuntimeCommand is ignored",
			gateway: &GatewayConfig{
				ContainerRuntime:        "podman",
				ContainerRuntimeCommand: "  ",
			},
			want: "podman",
		},
		{
			name: "ContainerRuntimeCommand with leading/trailing spaces is trimmed",
			gateway: &GatewayConfig{
				ContainerRuntimeCommand: "  myruntime  ",
			},
			want: "myruntime",
		},
		{
			name: "empty runtime falls back to default docker before custom command override",
			gateway: &GatewayConfig{
				ContainerRuntime:        "",
				ContainerRuntimeCommand: "customruntime",
			},
			want: "customruntime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configuredContainerRuntimeCommand(tt.gateway)
			assert.Equal(t, tt.want, got)
		})
	}
}
