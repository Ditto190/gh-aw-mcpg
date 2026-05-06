package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetGatewayPortFromEnv covers the MCP_GATEWAY_PORT parsing logic.
func TestGetGatewayPortFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantPort  int
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "not set",
			envValue:  "",
			wantErr:   true,
			errSubstr: "MCP_GATEWAY_PORT environment variable not set",
		},
		{
			name:      "not an integer",
			envValue:  "not-a-number",
			wantErr:   true,
			errSubstr: "invalid MCP_GATEWAY_PORT value",
		},
		{
			name:      "zero is out of range",
			envValue:  "0",
			wantErr:   true,
			errSubstr: "MCP_GATEWAY_PORT",
		},
		{
			name:      "negative port is out of range",
			envValue:  "-1",
			wantErr:   true,
			errSubstr: "MCP_GATEWAY_PORT",
		},
		{
			name:      "above max port is out of range",
			envValue:  "65536",
			wantErr:   true,
			errSubstr: "MCP_GATEWAY_PORT",
		},
		{
			name:     "valid port",
			envValue: "3000",
			wantPort: 3000,
		},
		{
			name:     "minimum valid port",
			envValue: "1",
			wantPort: 1,
		},
		{
			name:     "maximum valid port",
			envValue: "65535",
			wantPort: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("MCP_GATEWAY_PORT", tt.envValue)
			}

			port, err := GetGatewayPortFromEnv()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
				assert.Equal(t, 0, port)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPort, port)
			}
		})
	}
}

// TestGetGatewayDomainFromEnv covers the MCP_GATEWAY_DOMAIN lookup.
func TestGetGatewayDomainFromEnv(t *testing.T) {
	t.Run("returns empty string when not set", func(t *testing.T) {
		assert.Equal(t, "", GetGatewayDomainFromEnv())
	})

	t.Run("returns configured domain", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_DOMAIN", "gateway.example.com")
		assert.Equal(t, "gateway.example.com", GetGatewayDomainFromEnv())
	})

	t.Run("returns domain with port", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_DOMAIN", "gateway.example.com:8443")
		assert.Equal(t, "gateway.example.com:8443", GetGatewayDomainFromEnv())
	})
}

// TestGetGatewayAPIKeyFromEnv covers the MCP_GATEWAY_API_KEY lookup.
func TestGetGatewayAPIKeyFromEnv(t *testing.T) {
	t.Run("returns empty string when not set", func(t *testing.T) {
		assert.Equal(t, "", GetGatewayAPIKeyFromEnv())
	})

	t.Run("returns configured api key", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_API_KEY", "secret-api-key-abc123")
		assert.Equal(t, "secret-api-key-abc123", GetGatewayAPIKeyFromEnv())
	})
}

// TestGetGatewayToolTimeoutFromEnv covers the MCP_GATEWAY_TOOL_TIMEOUT parsing logic.
func TestGetGatewayToolTimeoutFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantVal   int
		wantOK    bool
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "not set returns false with no error",
			wantVal: 0,
			wantOK:  false,
		},
		{
			name:      "not an integer",
			envValue:  "not-a-number",
			wantErr:   true,
			errSubstr: "invalid MCP_GATEWAY_TOOL_TIMEOUT value",
		},
		{
			name:      "below minimum (9)",
			envValue:  "9",
			wantErr:   true,
			errSubstr: "MCP_GATEWAY_TOOL_TIMEOUT",
		},
		{
			name:      "zero is below minimum",
			envValue:  "0",
			wantErr:   true,
			errSubstr: "MCP_GATEWAY_TOOL_TIMEOUT",
		},
		{
			name:      "negative is below minimum",
			envValue:  "-1",
			wantErr:   true,
			errSubstr: "MCP_GATEWAY_TOOL_TIMEOUT",
		},
		{
			name:    "minimum valid value (10)",
			envValue: "10",
			wantVal: 10,
			wantOK:  true,
		},
		{
			name:    "default timeout value (60)",
			envValue: "60",
			wantVal: 60,
			wantOK:  true,
		},
		{
			name:    "large timeout value",
			envValue: "600",
			wantVal: 600,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("MCP_GATEWAY_TOOL_TIMEOUT", tt.envValue)
			}

			val, ok, err := GetGatewayToolTimeoutFromEnv()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
				assert.Equal(t, 0, val)
				assert.False(t, ok)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOK, ok)
				assert.Equal(t, tt.wantVal, val)
			}
		})
	}
}

// TestToolTimeoutEnvOrDefault covers the internal fallback helper.
func TestToolTimeoutEnvOrDefault(t *testing.T) {
	t.Run("returns DefaultToolTimeout when env var not set", func(t *testing.T) {
		assert.Equal(t, DefaultToolTimeout, toolTimeoutEnvOrDefault())
	})

	t.Run("returns DefaultToolTimeout when env var is invalid", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_TOOL_TIMEOUT", "bad-value")
		assert.Equal(t, DefaultToolTimeout, toolTimeoutEnvOrDefault())
	})

	t.Run("returns DefaultToolTimeout when env var is below minimum", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_TOOL_TIMEOUT", "5")
		assert.Equal(t, DefaultToolTimeout, toolTimeoutEnvOrDefault())
	})

	t.Run("returns configured timeout when valid", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_TOOL_TIMEOUT", "120")
		assert.Equal(t, 120, toolTimeoutEnvOrDefault())
	})

	t.Run("returns minimum valid timeout (10)", func(t *testing.T) {
		t.Setenv("MCP_GATEWAY_TOOL_TIMEOUT", "10")
		assert.Equal(t, 10, toolTimeoutEnvOrDefault())
	})
}
