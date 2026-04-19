package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateOpenTelemetryConfig tests all branches of validateOpenTelemetryConfig.
// The function validates W3C traceId/spanId formats and enforces HTTPS when required.
func TestValidateOpenTelemetryConfig(t *testing.T) {
	validTraceID := "4bf92f3577b34da6a3ce929d0e0e4736" // 32 lowercase hex chars
	validSpanID := "00f067aa0ba902b7"                  // 16 lowercase hex chars

	tests := []struct {
		name         string
		cfg          *TracingConfig
		enforceHTTPS bool
		wantErr      bool
		errContains  string
	}{
		// nil config — always a no-op
		{
			name:         "nil config returns nil",
			cfg:          nil,
			enforceHTTPS: true,
			wantErr:      false,
		},
		{
			name:         "nil config with enforceHTTPS false returns nil",
			cfg:          nil,
			enforceHTTPS: false,
			wantErr:      false,
		},

		// enforceHTTPS branch: endpoint required and must be HTTPS
		{
			name:         "missing endpoint when enforceHTTPS is true",
			cfg:          &TracingConfig{},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "endpoint",
		},
		{
			name: "http endpoint rejected when enforceHTTPS is true",
			cfg: &TracingConfig{
				Endpoint: "http://otel-collector.example.com",
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "HTTPS",
		},
		{
			name: "non-URL endpoint rejected when enforceHTTPS is true",
			cfg: &TracingConfig{
				Endpoint: "otel-collector.example.com",
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "HTTPS",
		},
		{
			name: "https endpoint accepted when enforceHTTPS is true",
			cfg: &TracingConfig{
				Endpoint: "https://otel-collector.example.com",
			},
			enforceHTTPS: true,
			wantErr:      false,
		},

		// enforceHTTPS=false: endpoint checks are skipped entirely
		{
			name:         "missing endpoint allowed when enforceHTTPS is false",
			cfg:          &TracingConfig{},
			enforceHTTPS: false,
			wantErr:      false,
		},
		{
			name: "http endpoint allowed when enforceHTTPS is false",
			cfg: &TracingConfig{
				Endpoint: "http://otel-collector.internal",
			},
			enforceHTTPS: false,
			wantErr:      false,
		},

		// traceId validation
		{
			name: "valid traceId passes",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  validTraceID,
			},
			enforceHTTPS: true,
			wantErr:      false,
		},
		{
			name: "traceId too short is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  "4bf92f3577b34da6a3ce929d0e0e473", // 31 chars
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "traceId",
		},
		{
			name: "traceId too long is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  "4bf92f3577b34da6a3ce929d0e0e47360", // 33 chars
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "traceId",
		},
		{
			name: "traceId with uppercase hex is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  "4BF92F3577B34DA6A3CE929D0E0E4736", // uppercase
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "traceId",
		},
		{
			name: "traceId with non-hex characters is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  "4bf92f3577b34da6a3ce929d0e0e47zz", // non-hex 'z'
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "traceId",
		},
		{
			name: "all-zero traceId is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  "00000000000000000000000000000000", // 32 zeros
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "traceId",
		},

		// spanId validation
		{
			name: "valid spanId with valid traceId passes",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  validTraceID,
				SpanID:   validSpanID,
			},
			enforceHTTPS: true,
			wantErr:      false,
		},
		{
			name: "spanId too short is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  validTraceID,
				SpanID:   "00f067aa0ba902b", // 15 chars
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "spanId",
		},
		{
			name: "spanId too long is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  validTraceID,
				SpanID:   "00f067aa0ba902b700", // 18 chars
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "spanId",
		},
		{
			name: "spanId with uppercase hex is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  validTraceID,
				SpanID:   "00F067AA0BA902B7", // uppercase
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "spanId",
		},
		{
			name: "all-zero spanId is rejected",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				TraceID:  validTraceID,
				SpanID:   "0000000000000000", // 16 zeros
			},
			enforceHTTPS: true,
			wantErr:      true,
			errContains:  "spanId",
		},

		// spanId without traceId: warning only, not an error
		{
			name: "spanId without traceId is a warning not an error",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
				SpanID:   validSpanID,
			},
			enforceHTTPS: true,
			wantErr:      false,
		},

		// fully valid config with all fields set
		{
			name: "fully valid config with traceId and spanId",
			cfg: &TracingConfig{
				Endpoint:    "https://otel.example.com",
				TraceID:     validTraceID,
				SpanID:      validSpanID,
				ServiceName: "mcp-gateway",
				Headers:     "Authorization=Bearer token123",
			},
			enforceHTTPS: true,
			wantErr:      false,
		},

		// valid config with only endpoint (no traceId or spanId)
		{
			name: "valid config with endpoint only",
			cfg: &TracingConfig{
				Endpoint: "https://otel.example.com",
			},
			enforceHTTPS: true,
			wantErr:      false,
		},

		// traceId validation skipped when enforceHTTPS=false and endpoint missing
		{
			name: "invalid traceId still rejected when enforceHTTPS is false",
			cfg: &TracingConfig{
				TraceID: "not-a-valid-trace-id",
			},
			enforceHTTPS: false,
			wantErr:      true,
			errContains:  "traceId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOpenTelemetryConfig(tt.cfg, tt.enforceHTTPS)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateTrustedBots tests the trustedBots list validation (spec §4.1.3.4).
func TestValidateTrustedBots(t *testing.T) {
	tests := []struct {
		name        string
		bots        []string
		wantErr     bool
		errContains string
	}{
		{
			name:    "nil bots is valid",
			bots:    nil,
			wantErr: false,
		},
		{
			name:        "empty slice is rejected",
			bots:        []string{},
			wantErr:     true,
			errContains: "non-empty",
		},
		{
			name:    "single valid bot name",
			bots:    []string{"github-actions[bot]"},
			wantErr: false,
		},
		{
			name:    "multiple valid bot names",
			bots:    []string{"dependabot[bot]", "github-actions[bot]", "renovate[bot]"},
			wantErr: false,
		},
		{
			name:        "empty string bot is rejected",
			bots:        []string{"valid-bot", ""},
			wantErr:     true,
			errContains: "non-empty string",
		},
		{
			name:        "whitespace-only bot is rejected",
			bots:        []string{"   "},
			wantErr:     true,
			errContains: "non-empty string",
		},
		{
			name:        "empty string at index 0 is rejected",
			bots:        []string{""},
			wantErr:     true,
			errContains: "trusted_bots[0]",
		},
		{
			name:        "empty string at index 1 is rejected with index in message",
			bots:        []string{"valid-bot", ""},
			wantErr:     true,
			errContains: "trusted_bots[1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTrustedBots(tt.bots)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateCustomSchemas tests custom schema type name and URL validation.
func TestValidateCustomSchemas(t *testing.T) {
	tests := []struct {
		name          string
		customSchemas map[string]interface{}
		wantErr       bool
		errContains   string
	}{
		{
			name:          "nil customSchemas is valid",
			customSchemas: nil,
			wantErr:       false,
		},
		{
			name:          "empty customSchemas is valid",
			customSchemas: map[string]interface{}{},
			wantErr:       false,
		},
		{
			name: "valid custom type with https schema URL",
			customSchemas: map[string]interface{}{
				"my-custom-type": "https://example.com/schema.json",
			},
			wantErr: false,
		},
		{
			name: "valid custom type with empty schema URL (skip validation)",
			customSchemas: map[string]interface{}{
				"my-custom-type": "",
			},
			wantErr: false,
		},
		{
			name: "valid custom type with nil schema value",
			customSchemas: map[string]interface{}{
				"my-custom-type": nil,
			},
			wantErr: false,
		},
		{
			name: "reserved type 'stdio' is rejected",
			customSchemas: map[string]interface{}{
				"stdio": "https://example.com/schema.json",
			},
			wantErr:     true,
			errContains: "stdio",
		},
		{
			name: "reserved type 'http' is rejected",
			customSchemas: map[string]interface{}{
				"http": "https://example.com/schema.json",
			},
			wantErr:     true,
			errContains: "http",
		},
		{
			name: "non-HTTPS schema URL is rejected",
			customSchemas: map[string]interface{}{
				"my-type": "http://example.com/schema.json",
			},
			wantErr:     true,
			errContains: "HTTPS",
		},
		{
			name: "schema URL without protocol is rejected",
			customSchemas: map[string]interface{}{
				"my-type": "example.com/schema.json",
			},
			wantErr:     true,
			errContains: "HTTPS",
		},
		{
			name: "multiple valid custom types",
			customSchemas: map[string]interface{}{
				"type-a": "https://example.com/schema-a.json",
				"type-b": "https://example.com/schema-b.json",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCustomSchemas(tt.customSchemas)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateGuardPolicies tests that guard policy validation iterates over
// all guards and delegates to ValidateGuardPolicy for non-nil policies.
func TestValidateGuardPolicies(t *testing.T) {
	validPolicy := &GuardPolicy{
		AllowOnly: &AllowOnlyPolicy{
			Repos:        "public",
			MinIntegrity: "none",
		},
	}

	tests := []struct {
		name        string
		cfg         *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "no guards passes",
			cfg: &Config{
				Guards: nil,
			},
			wantErr: false,
		},
		{
			name: "empty guards map passes",
			cfg: &Config{
				Guards: map[string]*GuardConfig{},
			},
			wantErr: false,
		},
		{
			name: "guard with nil config is skipped",
			cfg: &Config{
				Guards: map[string]*GuardConfig{
					"my-guard": nil,
				},
			},
			wantErr: false,
		},
		{
			name: "guard with nil policy is skipped",
			cfg: &Config{
				Guards: map[string]*GuardConfig{
					"my-guard": {
						Type:   "wasm",
						Policy: nil,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "guard with valid policy passes",
			cfg: &Config{
				Guards: map[string]*GuardConfig{
					"my-guard": {
						Type:   "wasm",
						Policy: validPolicy,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "guard with invalid policy returns error mentioning guard name",
			cfg: &Config{
				Guards: map[string]*GuardConfig{
					"bad-guard": {
						Type:   "wasm",
						Policy: &GuardPolicy{}, // empty policy is invalid
					},
				},
			},
			wantErr:     true,
			errContains: "bad-guard",
		},
		{
			name: "multiple guards: one invalid returns error",
			cfg: &Config{
				Guards: map[string]*GuardConfig{
					"good-guard": {
						Type:   "wasm",
						Policy: validPolicy,
					},
					"empty-policy-guard": {
						Type:   "wasm",
						Policy: &GuardPolicy{}, // invalid
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGuardPolicies(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateGatewayConfig_OpenTelemetry adds OTel-specific cases to validateGatewayConfig
// coverage that are not exercised by the main TestValidateGatewayConfig table.
func TestValidateGatewayConfig_OpenTelemetry(t *testing.T) {
	validTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	validSpanID := "00f067aa0ba902b7"

	tests := []struct {
		name        string
		gateway     *StdinGatewayConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid opentelemetry section passes",
			gateway: &StdinGatewayConfig{
				OpenTelemetry: &StdinOpenTelemetryConfig{
					Endpoint: "https://otel.example.com",
				},
			},
			wantErr: false,
		},
		{
			name: "opentelemetry with traceId and spanId passes",
			gateway: &StdinGatewayConfig{
				OpenTelemetry: &StdinOpenTelemetryConfig{
					Endpoint: "https://otel.example.com",
					TraceID:  validTraceID,
					SpanID:   validSpanID,
				},
			},
			wantErr: false,
		},
		{
			name: "opentelemetry missing endpoint fails",
			gateway: &StdinGatewayConfig{
				OpenTelemetry: &StdinOpenTelemetryConfig{},
			},
			wantErr:     true,
			errContains: "endpoint",
		},
		{
			name: "opentelemetry with http endpoint fails",
			gateway: &StdinGatewayConfig{
				OpenTelemetry: &StdinOpenTelemetryConfig{
					Endpoint: "http://otel.example.com",
				},
			},
			wantErr:     true,
			errContains: "HTTPS",
		},
		{
			name: "opentelemetry with invalid traceId fails",
			gateway: &StdinGatewayConfig{
				OpenTelemetry: &StdinOpenTelemetryConfig{
					Endpoint: "https://otel.example.com",
					TraceID:  "invalid",
				},
			},
			wantErr:     true,
			errContains: "traceId",
		},
		{
			name: "opentelemetry with all-zero traceId fails",
			gateway: &StdinGatewayConfig{
				OpenTelemetry: &StdinOpenTelemetryConfig{
					Endpoint: "https://otel.example.com",
					TraceID:  "00000000000000000000000000000000",
				},
			},
			wantErr:     true,
			errContains: "traceId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGatewayConfig(tt.gateway)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateGatewayConfig_TrustedBots adds trustedBots-specific cases to validateGatewayConfig
// coverage that are not exercised by the main TestValidateGatewayConfig table.
func TestValidateGatewayConfig_TrustedBots(t *testing.T) {
	tests := []struct {
		name        string
		gateway     *StdinGatewayConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid trustedBots list passes",
			gateway: &StdinGatewayConfig{
				TrustedBots: []string{"github-actions[bot]", "dependabot[bot]"},
			},
			wantErr: false,
		},
		{
			name: "empty trustedBots list is rejected",
			gateway: &StdinGatewayConfig{
				TrustedBots: []string{},
			},
			wantErr:     true,
			errContains: "non-empty",
		},
		{
			name: "trustedBots with empty string entry is rejected",
			gateway: &StdinGatewayConfig{
				TrustedBots: []string{"valid-bot", ""},
			},
			wantErr:     true,
			errContains: "non-empty string",
		},
		{
			name: "nil trustedBots (not set) is valid",
			gateway: &StdinGatewayConfig{
				TrustedBots: nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGatewayConfig(tt.gateway)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.ErrorContains(t, err, tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
