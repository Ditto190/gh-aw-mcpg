package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/config"
)

func TestDefaultTracingServiceName_IsCorrect(t *testing.T) {
	// Verify the default constant value hasn't changed unexpectedly.
	// "mcp-gateway" is the canonical service name used in OTLP traces.
	assert.Equal(t, "mcp-gateway", config.DefaultTracingServiceName,
		"DefaultTracingServiceName constant should remain 'mcp-gateway'")
}

// TestEnsureTracingConfig_WhenNil verifies that ensureTracingConfig initializes
// a new TracingConfig when cfg.Gateway.Tracing is nil.
func TestEnsureTracingConfig_WhenNil(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{},
	}
	require.Nil(t, cfg.Gateway.Tracing, "Tracing should start nil")

	tc := ensureTracingConfig(cfg)

	require.NotNil(t, tc, "ensureTracingConfig should return a non-nil TracingConfig")
	assert.Same(t, cfg.Gateway.Tracing, tc, "cfg.Gateway.Tracing should point to the returned config")
}

// TestEnsureTracingConfig_WhenNotNil verifies that ensureTracingConfig returns
// the existing TracingConfig without replacing it.
func TestEnsureTracingConfig_WhenNotNil(t *testing.T) {
	existing := &config.TracingConfig{Endpoint: "http://collector:4318"}
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{Tracing: existing},
	}

	tc := ensureTracingConfig(cfg)

	assert.Same(t, existing, tc, "ensureTracingConfig should return the existing TracingConfig unchanged")
	assert.Equal(t, "http://collector:4318", tc.Endpoint, "Endpoint should not be modified")
}

// TestEnsureTracingConfig_WithInitializedGateway verifies that callers must
// initialize cfg.Gateway before calling ensureTracingConfig, and that with a
// minimal Gateway in place the function returns a usable TracingConfig.
func TestEnsureTracingConfig_WithInitializedGateway(t *testing.T) {
	cfg := &config.Config{}
	assert.NotPanics(t, func() {
		// ensureTracingConfig dereferences cfg.Gateway.Tracing, so callers must
		// provide a non-nil Gateway before invoking it.
		cfg.Gateway = &config.GatewayConfig{}
		tc := ensureTracingConfig(cfg)
		assert.NotNil(t, tc)
	})
}

// TestRegisterTracingFlags_DefaultsWithNoEnv verifies that when neither
// OTEL_EXPORTER_OTLP_ENDPOINT nor OTEL_SERVICE_NAME are set, the flags use
// their built-in defaults (empty endpoint, "mcp-gateway" service name, 1.0 sample rate).
func TestRegisterTracingFlags_DefaultsWithNoEnv(t *testing.T) {
	originalEndpoint, hadEndpoint := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	originalService, hadService := os.LookupEnv("OTEL_SERVICE_NAME")

	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	os.Unsetenv("OTEL_SERVICE_NAME")
	t.Cleanup(func() {
		if hadEndpoint {
			os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", originalEndpoint)
		} else {
			os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if hadService {
			os.Setenv("OTEL_SERVICE_NAME", originalService)
		} else {
			os.Unsetenv("OTEL_SERVICE_NAME")
		}
	})

	cmd := &cobra.Command{Use: "test"}
	var endpoint, service string
	var sampleRate float64

	registerTracingFlags(cmd, &endpoint, &service, &sampleRate,
		"endpoint help", "service help", "sample help")

	actualEndpoint, err := cmd.Flags().GetString("otlp-endpoint")
	require.NoError(t, err)
	assert.Empty(t, actualEndpoint, "otlp-endpoint should default to empty when env var is not set")

	actualService, err := cmd.Flags().GetString("otlp-service-name")
	require.NoError(t, err)
	assert.Equal(t, config.DefaultTracingServiceName, actualService,
		"otlp-service-name should default to DefaultTracingServiceName when env var is not set")

	actualSampleRate, err := cmd.Flags().GetFloat64("otlp-sample-rate")
	require.NoError(t, err)
	assert.Equal(t, config.DefaultTracingSampleRate, actualSampleRate,
		"otlp-sample-rate should default to DefaultTracingSampleRate")
}

// TestApplyTracingFlags_ServiceNameEnvVar verifies that when OTEL_SERVICE_NAME is set
// in the environment (without an explicit --otlp-service-name CLI flag), applyFlagOrEnv
// propagates the env-var value into the tracing config — consistent with how
// OTEL_EXPORTER_OTLP_ENDPOINT is handled for the endpoint field.
func TestApplyTracingFlags_ServiceNameEnvVar(t *testing.T) {
	original, had := os.LookupEnv("OTEL_SERVICE_NAME")
	os.Setenv("OTEL_SERVICE_NAME", "env-service")
	t.Cleanup(func() {
		if had {
			os.Setenv("OTEL_SERVICE_NAME", original)
		} else {
			os.Unsetenv("OTEL_SERVICE_NAME")
		}
	})

	cmd := &cobra.Command{Use: "test"}
	var endpoint, service string
	var sampleRate float64
	registerTracingFlags(cmd, &endpoint, &service, &sampleRate,
		"endpoint help", "service help", "sample help")
	// Simulate flag parsing without passing --otlp-service-name explicitly.
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}
	tc := ensureTracingConfig(cfg)
	applyFlagOrEnv(cmd, "otlp-service-name", &tc.ServiceName, service, config.DefaultTracingServiceName)

	assert.Equal(t, "env-service", cfg.Gateway.Tracing.ServiceName,
		"OTEL_SERVICE_NAME env var should override tracing config service name")
}

// TestApplyTracingFlags_ServiceNameDefaultDoesNotOverrideConfig verifies that when
// OTEL_SERVICE_NAME is not set, the built-in default does NOT overwrite a value
// already present in the config (e.g. from a TOML file).
func TestApplyTracingFlags_ServiceNameDefaultDoesNotOverrideConfig(t *testing.T) {
	original, had := os.LookupEnv("OTEL_SERVICE_NAME")
	os.Unsetenv("OTEL_SERVICE_NAME")
	t.Cleanup(func() {
		if had {
			os.Setenv("OTEL_SERVICE_NAME", original)
		} else {
			os.Unsetenv("OTEL_SERVICE_NAME")
		}
	})

	cmd := &cobra.Command{Use: "test"}
	var endpoint, service string
	var sampleRate float64
	registerTracingFlags(cmd, &endpoint, &service, &sampleRate,
		"endpoint help", "service help", "sample help")
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Tracing: &config.TracingConfig{ServiceName: "toml-service"},
		},
	}
	applyFlagOrEnv(cmd, "otlp-service-name", &cfg.Gateway.Tracing.ServiceName, service, config.DefaultTracingServiceName)

	assert.Equal(t, "toml-service", cfg.Gateway.Tracing.ServiceName,
		"TOML config service name should not be overwritten when env var is unset and flag is unchanged")
}

// newTracingTestCmd builds a fresh cobra.Command with tracing flags registered,
// resetting the package-level otlp* flag variables used by applyTracingOverrides.
// It restores the previous package-level values via t.Cleanup.
func newTracingTestCmd(t *testing.T) *cobra.Command {
	t.Helper()

	origEndpoint, origService, origSampleRate := otlpEndpoint, otlpServiceName, otlpSampleRate
	t.Cleanup(func() {
		otlpEndpoint, otlpServiceName, otlpSampleRate = origEndpoint, origService, origSampleRate
	})

	otlpEndpoint = ""
	otlpServiceName = config.DefaultTracingServiceName
	otlpSampleRate = config.DefaultTracingSampleRate

	cmd := &cobra.Command{Use: "test"}
	registerTracingFlags(cmd, &otlpEndpoint, &otlpServiceName, &otlpSampleRate,
		"endpoint help", "service help", "sample help")
	return cmd
}

// TestApplyTracingOverrides_NoOverridesLeavesTracingNil verifies that when no
// CLI flags are changed, no env vars override the defaults, and cfg.Gateway.Tracing
// starts nil, applyTracingOverrides does not allocate a TracingConfig.
func TestApplyTracingOverrides_NoOverridesLeavesTracingNil(t *testing.T) {
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}
	applyTracingOverrides(cmd, cfg)

	assert.Nil(t, cfg.Gateway.Tracing, "Tracing config should remain nil when nothing overrides the defaults")
}

// TestApplyTracingOverrides_EndpointFlagInitializesConfig verifies that setting
// --otlp-endpoint triggers TracingConfig initialization and propagates the value.
func TestApplyTracingOverrides_EndpointFlagInitializesConfig(t *testing.T) {
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--otlp-endpoint", "http://collector:4318"}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}
	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "http://collector:4318", cfg.Gateway.Tracing.Endpoint)
}

// TestApplyTracingOverrides_ServiceNameFlag verifies that setting
// --otlp-service-name propagates through to the tracing config.
func TestApplyTracingOverrides_ServiceNameFlag(t *testing.T) {
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--otlp-service-name", "custom-service"}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}
	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "custom-service", cfg.Gateway.Tracing.ServiceName)
}

// TestApplyTracingOverrides_SampleRateFlag verifies that setting
// --otlp-sample-rate stores a pointer to the sample rate on the tracing config.
func TestApplyTracingOverrides_SampleRateFlag(t *testing.T) {
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--otlp-sample-rate", "0.5"}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}
	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	require.NotNil(t, cfg.Gateway.Tracing.SampleRate)
	assert.InDelta(t, 0.5, *cfg.Gateway.Tracing.SampleRate, 0.0001)
}

// TestApplyTracingOverrides_EndpointEnvVar verifies that a non-default value
// coming from OTEL_EXPORTER_OTLP_ENDPOINT (without an explicit CLI flag) still
// triggers config initialization and override, mirroring flag behavior.
func TestApplyTracingOverrides_EndpointEnvVar(t *testing.T) {
	original, had := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://env-collector:4318")
	t.Cleanup(func() {
		if had {
			os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", original)
		} else {
			os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		}
	})

	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}
	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "http://env-collector:4318", cfg.Gateway.Tracing.Endpoint)
}

// TestApplyTracingOverrides_PreExistingTracingConfigPreserved verifies that when
// cfg.Gateway.Tracing is already set (e.g. loaded from TOML) and no flags/env vars
// override it, applyTracingOverrides still initializes/keeps the config without
// clobbering existing fields.
func TestApplyTracingOverrides_PreExistingTracingConfigPreserved(t *testing.T) {
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Tracing: &config.TracingConfig{Endpoint: "http://toml-endpoint:4318", ServiceName: "toml-service"},
		},
	}
	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "http://toml-endpoint:4318", cfg.Gateway.Tracing.Endpoint,
		"existing endpoint should be preserved when no flag/env override is present")
	assert.Equal(t, "toml-service", cfg.Gateway.Tracing.ServiceName,
		"existing service name should be preserved when no flag/env override is present")
}

// TestApplyTracingOverrides_NilGateway verifies that applyTracingOverrides does not
// panic when cfg.Gateway is nil and no overrides are present (shouldInitTracingConfig
// stays false because the nil-Gateway branch of the OR condition short-circuits).
func TestApplyTracingOverrides_NilGateway(t *testing.T) {
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{}
	assert.NotPanics(t, func() {
		applyTracingOverrides(cmd, cfg)
	})
	assert.Nil(t, cfg.Gateway, "Gateway should remain nil since no overrides were present")
}
