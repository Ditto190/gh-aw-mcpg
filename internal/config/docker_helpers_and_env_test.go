package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateContainerID_SecurityCritical verifies the security-critical container ID validation.
// Container IDs must be 12–64 lowercase hex characters (a-f, 0-9).
func TestValidateContainerID_SecurityCritical(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty string",
			id:      "",
			wantErr: true,
			errMsg:  "container ID is empty",
		},
		{
			name:    "valid 12-char short form",
			id:      "abc123def456",
			wantErr: false,
		},
		{
			name:    "valid 64-char full form",
			id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			wantErr: false,
		},
		{
			name:    "valid 32-char intermediate",
			id:      "deadbeefcafe1234deadbeefcafe1234",
			wantErr: false,
		},
		{
			name:    "uppercase letters rejected",
			id:      "ABC123DEF456",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "mixed case rejected",
			id:      "abc123DEF456",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "non-hex characters rejected",
			id:      "xyz123abc456",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "hyphens rejected",
			id:      "abc123-def456",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "spaces rejected",
			id:      "abc123 def456",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "too short (11 chars)",
			id:      "abc123def45",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "too long (65 chars)",
			id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "slash injection attempt",
			id:      "abc123; rm -rf /",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "newline injection attempt",
			id:      "abc123def456\n",
			wantErr: true,
			errMsg:  "invalid characters",
		},
		{
			name:    "all zeros (valid)",
			id:      "000000000000",
			wantErr: false,
		},
		{
			name:    "all 'f' chars (valid)",
			id:      "ffffffffffff",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateContainerID(tt.id)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestExpandEnvArgs tests ExpandEnvArgs with various -e flag combinations.
func TestExpandEnvArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		envVars map[string]string
		want    []string
	}{
		{
			name: "nil input returns empty slice",
			args: nil,
			want: []string{},
		},
		{
			name: "empty input returns empty slice",
			args: []string{},
			want: []string{},
		},
		{
			name: "args without -e flag pass through unchanged",
			args: []string{"run", "--rm", "-i", "ghcr.io/org/image:latest"},
			want: []string{"run", "--rm", "-i", "ghcr.io/org/image:latest"},
		},
		{
			name:    "-e VAR_NAME is expanded when env var is set",
			args:    []string{"-e", "MY_TOKEN"},
			envVars: map[string]string{"MY_TOKEN": "secret-value"},
			want:    []string{"-e", "MY_TOKEN=secret-value"},
		},
		{
			name: "-e VAR_NAME is left unchanged when env var is not set",
			args: []string{"-e", "_DOCKERENV_TEST_TRULY_UNSET_XYZ999"},
			want: []string{"-e", "_DOCKERENV_TEST_TRULY_UNSET_XYZ999"},
		},
		{
			name:    "-e VAR=VALUE (already has =) is passed through unchanged",
			args:    []string{"-e", "MY_VAR=already-set"},
			envVars: map[string]string{"MY_VAR": "other-value"},
			want:    []string{"-e", "MY_VAR=already-set"},
		},
		{
			name: "-e at end of args (no following value) is passed through",
			args: []string{"run", "-e"},
			want: []string{"run", "-e"},
		},
		{
			name:    "multiple -e flags are all expanded",
			args:    []string{"-e", "TOKEN_A", "-e", "TOKEN_B"},
			envVars: map[string]string{"TOKEN_A": "val-a", "TOKEN_B": "val-b"},
			want:    []string{"-e", "TOKEN_A=val-a", "-e", "TOKEN_B=val-b"},
		},
		{
			name:    "mix of set and unset env vars in -e flags",
			args:    []string{"-e", "SET_VAR", "-e", "_DOCKERENV_TEST_TRULY_UNSET_XYZ999"},
			envVars: map[string]string{"SET_VAR": "value"},
			want:    []string{"-e", "SET_VAR=value", "-e", "_DOCKERENV_TEST_TRULY_UNSET_XYZ999"},
		},
		{
			name:    "realistic docker run command with env var expansion",
			args:    []string{"run", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN", "-i", "ghcr.io/github/github-mcp-server:latest"},
			envVars: map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_abc123"},
			want:    []string{"run", "--rm", "-e", "GITHUB_PERSONAL_ACCESS_TOKEN=ghp_abc123", "-i", "ghcr.io/github/github-mcp-server:latest"},
		},
		{
			name: "-e VAR where VAR is empty string is passed through unchanged",
			args: []string{"-e", ""},
			want: []string{"-e", ""},
		},
		{
			name:    "env var with empty value is expanded with empty value",
			args:    []string{"-e", "EMPTY_VAR"},
			envVars: map[string]string{"EMPTY_VAR": ""},
			want:    []string{"-e", "EMPTY_VAR="},
		},
		{
			name:    "env var value containing = is expanded correctly",
			args:    []string{"-e", "URL_VAR"},
			envVars: map[string]string{"URL_VAR": "https://example.com?key=val"},
			want:    []string{"-e", "URL_VAR=https://example.com?key=val"},
		},
		{
			name:    "non -e flags between expanded flags are preserved",
			args:    []string{"-e", "VAR_A", "--name", "mycontainer", "-e", "VAR_B"},
			envVars: map[string]string{"VAR_A": "alpha", "VAR_B": "beta"},
			want:    []string{"-e", "VAR_A=alpha", "--name", "mycontainer", "-e", "VAR_B=beta"},
		},
		{
			name:    "same var referenced multiple times is expanded each time",
			args:    []string{"-e", "SHARED_VAR", "-e", "SHARED_VAR"},
			envVars: map[string]string{"SHARED_VAR": "shared"},
			want:    []string{"-e", "SHARED_VAR=shared", "-e", "SHARED_VAR=shared"},
		},
		{
			name: "-e immediately followed by another -e flag where first -e var is unset",
			args: []string{"-e", "-e"},
			// The first "-e" tries to expand the second "-e" as a var name.
			// Since no env var named "-e" is set, the expansion is skipped.
			// Both "-e" args are emitted as-is.
			want: []string{"-e", "-e"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got := ExpandEnvArgs(tt.args)

			require.NotNil(t, got, "ExpandEnvArgs should never return nil")
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestExpandEnvArgs_DoesNotMutateInput verifies that the original args slice
// is not modified by ExpandEnvArgs.
func TestExpandEnvArgs_DoesNotMutateInput(t *testing.T) {
	t.Setenv("MY_SECRET", "secret-value")

	original := []string{"-e", "MY_SECRET", "--rm"}
	// Make a copy to compare against after the call
	copyOfOriginal := make([]string, len(original))
	copy(copyOfOriginal, original)

	ExpandEnvArgs(original)

	assert.Equal(t, copyOfOriginal, original, "ExpandEnvArgs must not mutate the input slice")
}

// TestExpandEnvArgs_OutputIsIndependentOfInput verifies that modifications to
// the returned slice do not affect the original input.
func TestExpandEnvArgs_OutputIsIndependentOfInput(t *testing.T) {
	t.Setenv("SOME_VAR", "value")

	args := []string{"run", "-e", "SOME_VAR"}
	result := ExpandEnvArgs(args)

	// Modifying the result should not affect the original
	result[0] = "MODIFIED"
	assert.Equal(t, "run", args[0], "Modifying result should not affect original slice")
}

// TestGetGatewayPortFromEnv tests the env-based gateway port parsing.
func TestGetGatewayPortFromEnv_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		envSet   bool
		wantPort int
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "env var not set",
			envSet:  false,
			wantErr: true,
			errMsg:  "MCP_GATEWAY_PORT environment variable not set",
		},
		{
			name:     "env var set to empty string",
			envValue: "",
			envSet:   true,
			wantErr:  true,
			errMsg:   "MCP_GATEWAY_PORT environment variable not set",
		},
		{
			name:     "invalid integer value",
			envValue: "not-a-number",
			envSet:   true,
			wantErr:  true,
			errMsg:   "invalid MCP_GATEWAY_PORT value",
		},
		{
			name:     "port zero (out of range)",
			envValue: "0",
			envSet:   true,
			wantErr:  true,
		},
		{
			name:     "port 65536 (out of range)",
			envValue: "65536",
			envSet:   true,
			wantErr:  true,
		},
		{
			name:     "negative port",
			envValue: "-1",
			envSet:   true,
			wantErr:  true,
		},
		{
			name:     "valid port 3000",
			envValue: "3000",
			envSet:   true,
			wantPort: 3000,
			wantErr:  false,
		},
		{
			name:     "valid port 1",
			envValue: "1",
			envSet:   true,
			wantPort: 1,
			wantErr:  false,
		},
		{
			name:     "valid port 65535 (max)",
			envValue: "65535",
			envSet:   true,
			wantPort: 65535,
			wantErr:  false,
		},
		{
			name:     "valid port 8080",
			envValue: "8080",
			envSet:   true,
			wantPort: 8080,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv("MCP_GATEWAY_PORT", tt.envValue)
			} else {
				os.Unsetenv("MCP_GATEWAY_PORT")
			}

			port, err := GetGatewayPortFromEnv()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Equal(t, 0, port)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPort, port)
			}
		})
	}
}
