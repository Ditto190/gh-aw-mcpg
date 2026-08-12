package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/config"
)

// resetTracingGlobals saves the current values of the package-level tracing
// flag variables and returns a restore function. applyTracingOverrides reads
// these globals directly (they are populated by registerTracingFlags), so
// tests that exercise different combinations of CLI/env overrides must
// isolate themselves from one another.
func resetTracingGlobals(t *testing.T) {
	t.Helper()
	origEndpoint := otlpEndpoint
	origServiceName := otlpServiceName
	origSampleRate := otlpSampleRate
	t.Cleanup(func() {
		otlpEndpoint = origEndpoint
		otlpServiceName = origServiceName
		otlpSampleRate = origSampleRate
	})
	otlpEndpoint = ""
	otlpServiceName = config.DefaultTracingServiceName
	otlpSampleRate = config.DefaultTracingSampleRate
}

// newTracingTestCmd creates a cobra.Command with the tracing flags registered
// against the package-level globals, mirroring how the real root command
// wires them up via init()/RegisterFlag.
func newTracingTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	cmd := &cobra.Command{Use: "test"}
	registerTracingFlags(cmd, &otlpEndpoint, &otlpServiceName, &otlpSampleRate,
		"endpoint help", "service help", "sample help")
	return cmd
}

// TestApplyTracingOverrides_NilGatewayNoOverrides verifies that when cfg.Gateway
// is nil and no CLI flags or env vars override the tracing defaults, the
// function is a no-op and does not panic (exercising the short-circuit in the
// shouldInitTracingConfig condition).
func TestApplyTracingOverrides_NilGatewayNoOverrides(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{}

	assert.NotPanics(t, func() {
		applyTracingOverrides(cmd, cfg)
	})
	assert.Nil(t, cfg.Gateway, "cfg.Gateway should remain nil when there is nothing to apply")
}

// TestApplyTracingOverrides_GatewayNoTracingNoOverrides verifies that when
// cfg.Gateway is set but Tracing is nil, and no flags/env vars override the
// defaults, applyTracingOverrides leaves cfg.Gateway.Tracing nil (shouldInit
// evaluates to false).
func TestApplyTracingOverrides_GatewayNoTracingNoOverrides(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}

	applyTracingOverrides(cmd, cfg)

	assert.Nil(t, cfg.Gateway.Tracing, "Tracing should remain nil when nothing overrides the defaults")
}

// TestApplyTracingOverrides_ExistingTracingConfigPreserved verifies that when
// cfg.Gateway.Tracing is already populated (e.g. loaded from a TOML file) and
// there are no CLI/env overrides, shouldInitTracingConfig is true (due to the
// existing Tracing config) but applyFlagOrEnv does not clobber the existing
// values since the flags were not changed and the globals equal their
// defaults.
func TestApplyTracingOverrides_ExistingTracingConfigPreserved(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))

	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			Tracing: &config.TracingConfig{
				Endpoint:    "http://toml-endpoint:4318",
				ServiceName: "toml-service",
			},
		},
	}

	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "http://toml-endpoint:4318", cfg.Gateway.Tracing.Endpoint,
		"existing TOML endpoint should not be overwritten")
	assert.Equal(t, "toml-service", cfg.Gateway.Tracing.ServiceName,
		"existing TOML service name should not be overwritten")
	assert.Nil(t, cfg.Gateway.Tracing.SampleRate, "sample rate should remain unset")
}

// TestApplyTracingOverrides_EnvVarOverride verifies that when the
// otlp-service-name global differs from its default (simulating an
// OTEL_SERVICE_NAME env var override applied by registerTracingFlags), the
// tracing config is initialized and the service name override is applied,
// even though cfg.Gateway.Tracing started nil.
func TestApplyTracingOverrides_EnvVarOverride(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{}))
	// Simulate an env-var-driven default that differs from the built-in default,
	// as would happen if OTEL_SERVICE_NAME were set when registerTracingFlags ran.
	otlpServiceName = "env-service"

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}

	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing, "Tracing config should be initialized due to env override")
	assert.Equal(t, "env-service", cfg.Gateway.Tracing.ServiceName)
	assert.Empty(t, cfg.Gateway.Tracing.Endpoint, "endpoint should remain empty since it was not overridden")
}

// TestApplyTracingOverrides_CLIEndpointFlag verifies that explicitly passing
// --otlp-endpoint initializes the tracing config and applies the endpoint
// value, exercising the cmd.Flags().Changed("otlp-endpoint") branch.
func TestApplyTracingOverrides_CLIEndpointFlag(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--otlp-endpoint=http://cli-endpoint:4318"}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}

	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "http://cli-endpoint:4318", cfg.Gateway.Tracing.Endpoint)
	// otlp-service-name was not changed and its global equals the default, so
	// applyFlagOrEnv leaves ServiceName at its zero value (unset).
	assert.Empty(t, cfg.Gateway.Tracing.ServiceName)
}

// TestApplyTracingOverrides_CLIServiceNameFlag verifies that explicitly
// passing --otlp-service-name initializes the tracing config and applies the
// service name value, exercising the cmd.Flags().Changed("otlp-service-name")
// branch of the shouldInitTracingConfig condition.
func TestApplyTracingOverrides_CLIServiceNameFlag(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--otlp-service-name=cli-service"}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}

	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "cli-service", cfg.Gateway.Tracing.ServiceName)
	assert.Empty(t, cfg.Gateway.Tracing.Endpoint)
}

// TestApplyTracingOverrides_CLISampleRateFlag verifies that explicitly
// passing --otlp-sample-rate initializes the tracing config and sets
// SampleRate to a pointer holding the parsed value, exercising the
// cmd.Flags().Changed("otlp-sample-rate") branches in both
// shouldInitTracingConfig and the SampleRate assignment.
func TestApplyTracingOverrides_CLISampleRateFlag(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--otlp-sample-rate=0.42"}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}

	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	require.NotNil(t, cfg.Gateway.Tracing.SampleRate, "SampleRate pointer should be set when the flag is changed")
	assert.InDelta(t, 0.42, *cfg.Gateway.Tracing.SampleRate, 0.0001)
}

// TestApplyTracingOverrides_SampleRateNotSetWhenUnchanged verifies that when
// the --otlp-sample-rate flag is not explicitly passed, SampleRate remains
// nil even though other overrides trigger tracing config initialization.
func TestApplyTracingOverrides_SampleRateNotSetWhenUnchanged(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{"--otlp-endpoint=http://cli-endpoint:4318"}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}

	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Nil(t, cfg.Gateway.Tracing.SampleRate,
		"SampleRate should stay nil when --otlp-sample-rate was not explicitly set")
}

// TestApplyTracingOverrides_AllFlagsSetTogether verifies the combination of
// all three CLI flags being explicitly set at once, ensuring all three
// applyFlagOrEnv calls and the SampleRate assignment interact correctly.
func TestApplyTracingOverrides_AllFlagsSetTogether(t *testing.T) {
	resetTracingGlobals(t)
	cmd := newTracingTestCmd(t)
	require.NoError(t, cmd.ParseFlags([]string{
		"--otlp-endpoint=http://all-endpoint:4318",
		"--otlp-service-name=all-service",
		"--otlp-sample-rate=0.75",
	}))

	cfg := &config.Config{Gateway: &config.GatewayConfig{}}

	applyTracingOverrides(cmd, cfg)

	require.NotNil(t, cfg.Gateway.Tracing)
	assert.Equal(t, "http://all-endpoint:4318", cfg.Gateway.Tracing.Endpoint)
	assert.Equal(t, "all-service", cfg.Gateway.Tracing.ServiceName)
	require.NotNil(t, cfg.Gateway.Tracing.SampleRate)
	assert.InDelta(t, 0.75, *cfg.Gateway.Tracing.SampleRate, 0.0001)
}
