package cmd

import (
	"os"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDIFCMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{
			name:    "strict mode valid",
			mode:    "strict",
			wantErr: false,
		},
		{
			name:    "filter mode valid",
			mode:    "filter",
			wantErr: false,
		},
		{
			name:    "propagate mode valid",
			mode:    "propagate",
			wantErr: false,
		},
		{
			name:    "uppercase STRICT valid",
			mode:    "STRICT",
			wantErr: false,
		},
		{
			name:    "mixed case Filter valid",
			mode:    "Filter",
			wantErr: false,
		},
		{
			name:    "invalid mode",
			mode:    "invalid",
			wantErr: true,
		},
		{
			name:    "empty mode",
			mode:    "",
			wantErr: true,
		},
		{
			name:    "partial match should fail",
			mode:    "stric",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDIFCMode(tt.mode)
			if tt.wantErr {
				assert.Error(t, err, "expected error for mode %q", tt.mode)
				assert.Contains(t, err.Error(), "invalid DIFC mode")
			} else {
				assert.NoError(t, err, "unexpected error for mode %q", tt.mode)
			}
		})
	}
}

func TestGetDefaultEnableDIFC(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			name:     "no env var",
			envValue: "",
			want:     false,
		},
		{
			name:     "env var true",
			envValue: "true",
			want:     true,
		},
		{
			name:     "env var 1",
			envValue: "1",
			want:     true,
		},
		{
			name:     "env var yes",
			envValue: "yes",
			want:     true,
		},
		{
			name:     "env var on",
			envValue: "on",
			want:     true,
		},
		{
			name:     "env var TRUE uppercase",
			envValue: "TRUE",
			want:     true,
		},
		{
			name:     "env var false",
			envValue: "false",
			want:     false,
		},
		{
			name:     "env var invalid",
			envValue: "invalid",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env
			originalEnv := os.Getenv("MCP_GATEWAY_ENABLE_DIFC")
			defer func() {
				if originalEnv != "" {
					os.Setenv("MCP_GATEWAY_ENABLE_DIFC", originalEnv)
				} else {
					os.Unsetenv("MCP_GATEWAY_ENABLE_DIFC")
				}
			}()

			if tt.envValue != "" {
				os.Setenv("MCP_GATEWAY_ENABLE_DIFC", tt.envValue)
			} else {
				os.Unsetenv("MCP_GATEWAY_ENABLE_DIFC")
			}

			got := getDefaultEnableDIFC()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetDefaultDIFCMode(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "no env var returns strict",
			envValue: "",
			want:     "strict",
		},
		{
			name:     "env var strict",
			envValue: "strict",
			want:     "strict",
		},
		{
			name:     "env var filter",
			envValue: "filter",
			want:     "filter",
		},
		{
			name:     "env var propagate",
			envValue: "propagate",
			want:     "propagate",
		},
		{
			name:     "env var FILTER uppercase",
			envValue: "FILTER",
			want:     "filter",
		},
		{
			name:     "env var invalid falls back to strict",
			envValue: "invalid",
			want:     "strict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env
			originalEnv := os.Getenv("MCP_GATEWAY_DIFC_MODE")
			defer func() {
				if originalEnv != "" {
					os.Setenv("MCP_GATEWAY_DIFC_MODE", originalEnv)
				} else {
					os.Unsetenv("MCP_GATEWAY_DIFC_MODE")
				}
			}()

			if tt.envValue != "" {
				os.Setenv("MCP_GATEWAY_DIFC_MODE", tt.envValue)
			} else {
				os.Unsetenv("MCP_GATEWAY_DIFC_MODE")
			}

			got := getDefaultDIFCMode()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidDIFCModes(t *testing.T) {
	require := require.New(t)

	// Verify all expected modes are valid using isValidDIFCMode
	require.True(isValidDIFCMode(difc.ModeStrict), "strict should be valid")
	require.True(isValidDIFCMode(difc.ModeFilter), "filter should be valid")
	require.True(isValidDIFCMode(difc.ModePropagate), "propagate should be valid")

	// Verify ValidModes slice has 3 entries
	require.Len(difc.ValidModes, 3, "should only have 3 valid modes")
}

func TestGetDefaultConfigExtensions(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			name:     "no env var",
			envValue: "",
			want:     false,
		},
		{
			name:     "env var true",
			envValue: "true",
			want:     true,
		},
		{
			name:     "env var 1",
			envValue: "1",
			want:     true,
		},
		{
			name:     "env var false",
			envValue: "false",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := os.Getenv("MCP_GATEWAY_CONFIG_EXTENSIONS")
			defer func() {
				if originalEnv != "" {
					os.Setenv("MCP_GATEWAY_CONFIG_EXTENSIONS", originalEnv)
				} else {
					os.Unsetenv("MCP_GATEWAY_CONFIG_EXTENSIONS")
				}
			}()

			if tt.envValue != "" {
				os.Setenv("MCP_GATEWAY_CONFIG_EXTENSIONS", tt.envValue)
			} else {
				os.Unsetenv("MCP_GATEWAY_CONFIG_EXTENSIONS")
			}

			got := getDefaultConfigExtensions()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetDefaultSessionSecrecy(t *testing.T) {
	originalEnv := os.Getenv("MCP_GATEWAY_SESSION_SECRECY")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MCP_GATEWAY_SESSION_SECRECY", originalEnv)
		} else {
			os.Unsetenv("MCP_GATEWAY_SESSION_SECRECY")
		}
	}()

	// Test empty
	os.Unsetenv("MCP_GATEWAY_SESSION_SECRECY")
	assert.Equal(t, "", getDefaultSessionSecrecy())

	// Test with value
	os.Setenv("MCP_GATEWAY_SESSION_SECRECY", "secret,confidential")
	assert.Equal(t, "secret,confidential", getDefaultSessionSecrecy())
}

func TestGetDefaultSessionIntegrity(t *testing.T) {
	originalEnv := os.Getenv("MCP_GATEWAY_SESSION_INTEGRITY")
	defer func() {
		if originalEnv != "" {
			os.Setenv("MCP_GATEWAY_SESSION_INTEGRITY", originalEnv)
		} else {
			os.Unsetenv("MCP_GATEWAY_SESSION_INTEGRITY")
		}
	}()

	// Test empty
	os.Unsetenv("MCP_GATEWAY_SESSION_INTEGRITY")
	assert.Equal(t, "", getDefaultSessionIntegrity())

	// Test with value
	os.Setenv("MCP_GATEWAY_SESSION_INTEGRITY", "trusted,verified")
	assert.Equal(t, "trusted,verified", getDefaultSessionIntegrity())
}

func TestParseSessionLabels(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "empty string",
			input:  "",
			expect: nil,
		},
		{
			name:   "single label",
			input:  "secret",
			expect: []string{"secret"},
		},
		{
			name:   "multiple labels",
			input:  "secret,confidential,internal",
			expect: []string{"secret", "confidential", "internal"},
		},
		{
			name:   "labels with spaces",
			input:  "secret , confidential , internal",
			expect: []string{"secret", "confidential", "internal"},
		},
		{
			name:   "empty items filtered",
			input:  "secret,,confidential",
			expect: []string{"secret", "confidential"},
		},
		{
			name:   "only commas",
			input:  ",,",
			expect: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSessionLabels(tt.input)
			assert.Equal(t, tt.expect, result)
		})
	}
}
