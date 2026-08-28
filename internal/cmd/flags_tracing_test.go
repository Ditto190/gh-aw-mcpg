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

// resetTracingFlagGlobals resets the package-level tracing flag variables used by
// applyTracingOverrides to their zero/default values and restores them after the
// test, since these are shared mutable package state.
func resetTracingFlagGlobals(t *testing.T) {
	t.Helper()
	origEndpoint, origService, origSampleRate := otlpEndpoint, otlpServiceName, otlpSampleRate
	otlpEndpoint = ""
	otlpServiceName = config.DefaultTracingServiceName
	otlpSampleRate = config.DefaultTracingSampleRate
	t.Cleanup(func() {
		otlpEndpoint, otlpServiceName, otlpSampleRate = origEndpoint, origService, origSampleRate
	})
}

// TestApplyTracingOverrides covers every branch of applyTracingOverrides: the
// "no initialization needed" fast path, each individual trigger for initializing
// the tracing config (existing Tracing struct, --otlp-endpoint flag/value,
// --otlp-service-name flag/value, --otlp-sample-rate flag), and verifies that
// the overrides are actually copied into cfg.Gateway.Tracing.
func TestApplyTracingOverrides(t *testing.T) {
	newTestCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		var endpoint, service string
		var sampleRate float64
		registerTracingFlags(cmd, &endpoint, &service, &sampleRate, "endpoint help", "service help", "sample help")
		return cmd
	}

	t.Run("does nothing when no trigger is present", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{}))

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		assert.Nil(t, cfg.Gateway.Tracing, "Tracing config should remain nil when nothing triggers initialization")
	})

	t.Run("initializes when Gateway.Tracing already set", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{}))

		cfg := &config.Config{
			Gateway: &config.GatewayConfig{Tracing: &config.TracingConfig{Endpoint: "http://existing:4318"}},
		}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "http://existing:4318", cfg.Gateway.Tracing.Endpoint,
			"Existing endpoint should be preserved when no flag/env overrides it")
	})

	t.Run("initializes when --otlp-endpoint flag changed", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--otlp-endpoint=http://flag:4318"}))
		otlpEndpoint = "http://flag:4318"

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "http://flag:4318", cfg.Gateway.Tracing.Endpoint)
	})

	t.Run("initializes when otlpEndpoint var is non-empty without flag being marked changed", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{}))
		otlpEndpoint = "http://env:4318"

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "http://env:4318", cfg.Gateway.Tracing.Endpoint)
	})

	t.Run("initializes when --otlp-service-name flag changed", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--otlp-service-name=svc-a"}))
		otlpServiceName = "svc-a"

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "svc-a", cfg.Gateway.Tracing.ServiceName)
	})

	t.Run("initializes when otlpServiceName differs from default without flag changed", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{}))
		otlpServiceName = "non-default-service"

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "non-default-service", cfg.Gateway.Tracing.ServiceName)
	})

	t.Run("initializes when --otlp-sample-rate flag changed and copies the rate", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--otlp-sample-rate=0.5"}))
		otlpSampleRate = 0.5

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		require.NotNil(t, cfg.Gateway.Tracing.SampleRate)
		assert.InDelta(t, 0.5, *cfg.Gateway.Tracing.SampleRate, 0.0001)
	})

	t.Run("does not set SampleRate pointer when sample-rate flag unchanged", func(t *testing.T) {
		resetTracingFlagGlobals(t)
		cmd := newTestCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--otlp-endpoint=http://trigger:4318"}))
		otlpEndpoint = "http://trigger:4318"

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Nil(t, cfg.Gateway.Tracing.SampleRate,
			"SampleRate pointer should stay nil when --otlp-sample-rate was not explicitly set")
	})
}
