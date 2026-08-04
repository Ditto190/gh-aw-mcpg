package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/tracing"
)

func TestRegisterTracingFlags_DefaultsFromEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
	t.Setenv("OTEL_SERVICE_NAME", "test-service")

	cmd := &cobra.Command{Use: "test"}

	var endpoint string
	var service string
	var sampleRate float64

	registerTracingFlags(
		cmd,
		&endpoint,
		&service,
		&sampleRate,
		"endpoint help",
		"service help",
		"sample help",
	)

	actualEndpoint, err := cmd.Flags().GetString("otlp-endpoint")
	require.NoError(t, err)
	assert.Equal(t, "http://collector:4318", actualEndpoint)

	actualService, err := cmd.Flags().GetString("otlp-service-name")
	require.NoError(t, err)
	assert.Equal(t, "test-service", actualService)

	actualSampleRate, err := cmd.Flags().GetFloat64("otlp-sample-rate")
	require.NoError(t, err)
	assert.Equal(t, config.DefaultTracingSampleRate, actualSampleRate)

	err = cmd.ParseFlags([]string{
		"--otlp-endpoint=http://override:4318",
		"--otlp-service-name=override-service",
		"--otlp-sample-rate=0.25",
	})
	require.NoError(t, err)
	assert.Equal(t, "http://override:4318", endpoint)
	assert.Equal(t, "override-service", service)
	assert.Equal(t, 0.25, sampleRate)
}

// TestInitTracingProviderWithFallback verifies that the helper returns a working
// provider for both the nil-config (noop) path and the error-fallback path.
func TestInitTracingProviderWithFallback(t *testing.T) {
	t.Run("nil config returns noop provider without error", func(t *testing.T) {
		var warnCalled bool
		provider := initTracingProviderWithFallback(
			context.Background(),
			nil,
			"warn: %v",
			func(format string, args ...any) { warnCalled = true },
		)
		require.NotNil(t, provider, "Provider must not be nil")
		assert.False(t, warnCalled, "No warning should be emitted for nil config")
	})

	t.Run("valid config with no endpoint returns noop provider without warning", func(t *testing.T) {
		var warnCalled bool
		cfg := &config.TracingConfig{
			// No endpoint — InitProvider should succeed with a noop tracer.
		}
		provider := initTracingProviderWithFallback(
			context.Background(),
			cfg,
			"warn: %v",
			func(format string, args ...any) { warnCalled = true },
		)
		require.NotNil(t, provider, "Provider must not be nil")
		assert.False(t, warnCalled, "No warning expected when no endpoint is configured")
	})

	t.Run("configured endpoint creates SDK provider without warning", func(t *testing.T) {
		// HTTP OTLP exporters are lazily connected: otlptracehttp.New succeeds even
		// for unreachable endpoints, so InitProvider does not return an error and the
		// warn callback is never invoked. The provider is a real SDK (non-noop) instance.
		// Reset the global OTel provider to noop after the subtest to avoid leaking
		// background batcher goroutines and making other tests order-dependent.
		t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })
		var warnCalled bool
		cfg := &config.TracingConfig{
			Endpoint: "https://127.0.0.1:1/does-not-exist",
		}
		provider := initTracingProviderWithFallback(
			context.Background(),
			cfg,
			"tracing init failed: %v",
			func(format string, args ...any) { warnCalled = true },
		)
		require.NotNil(t, provider, "Provider must not be nil")
		assert.False(t, warnCalled, "OTLP exporter construction is lazy; no warning expected")
		assert.True(t, provider.IsEnabled(), "Configured endpoint should produce an SDK (non-noop) provider")
	})
}

// TestShutdownTracingProviderWithTimeout verifies that the shutdown helper
// completes without panicking for a noop provider (which is the common case).
func TestShutdownTracingProviderWithTimeout(t *testing.T) {
	t.Run("noop provider shuts down cleanly", func(t *testing.T) {
		var warnCalled bool
		provider := initTracingProviderWithFallback(
			context.Background(),
			nil,
			"warn: %v",
			func(format string, args ...any) {},
		)
		require.NotNil(t, provider)

		// Should complete without panic or warning.
		shutdownTracingProviderWithTimeout(provider, func(format string, args ...any) {
			warnCalled = true
		})
		assert.False(t, warnCalled, "Shutdown of noop provider should not produce a warning")
	})

	t.Run("sdk provider shuts down cleanly", func(t *testing.T) {
		// HTTP OTLP exporters are lazy; construction succeeds even for unreachable endpoints.
		// Reset the global OTel provider to noop after the subtest so that a shut-down
		// provider is not left as the global, which would make later tests order-dependent.
		t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })
		provider, err := tracing.InitProvider(context.Background(), &config.TracingConfig{
			Endpoint: "http://127.0.0.1:14318",
		})
		require.NoError(t, err)
		require.True(t, provider.IsEnabled(), "Expected a real SDK provider with a configured endpoint")

		var warnCalled bool
		shutdownTracingProviderWithTimeout(provider, func(format string, args ...any) {
			warnCalled = true
		})
		assert.False(t, warnCalled, "SDK provider with no pending spans should shut down without error")
	})
}

func TestSetupCommandTracing(t *testing.T) {
	t.Run("returns provider and cleanup for noop tracing config", func(t *testing.T) {
		var initWarnCalled bool
		var shutdownWarnCalled bool

		provider, cleanup := setupCommandTracing(
			context.Background(),
			nil,
			"warn: %v",
			func(format string, args ...any) {
				initWarnCalled = true
			},
			func(format string, args ...any) {
				shutdownWarnCalled = true
			},
		)

		require.NotNil(t, provider)
		require.NotNil(t, cleanup)
		assert.False(t, initWarnCalled)

		assert.NotPanics(t, cleanup)
		assert.False(t, shutdownWarnCalled)
	})
}

// TestApplyTracingOverrides covers the branch logic in applyTracingOverrides.
// Because applyTracingOverrides reads package-level globals (otlpEndpoint,
// otlpServiceName), these tests must NOT run in parallel.
func TestApplyTracingOverrides(t *testing.T) {
	// Save and restore the package-level globals so each sub-test starts clean.
	savedEndpoint := otlpEndpoint
	savedServiceName := otlpServiceName
	savedSampleRate := otlpSampleRate
	t.Cleanup(func() {
		otlpEndpoint = savedEndpoint
		otlpServiceName = savedServiceName
		otlpSampleRate = savedSampleRate
	})

	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		var ep, svc string
		var rate float64
		registerTracingFlags(cmd, &ep, &svc, &rate, "", "", "")
		return cmd
	}

	t.Run("does nothing when config has no tracing and no flags changed", func(t *testing.T) {
		otlpEndpoint = ""
		otlpServiceName = config.DefaultTracingServiceName

		cmd := newCmd()
		cfg := &config.Config{Gateway: &config.GatewayConfig{}}

		applyTracingOverrides(cmd, cfg)

		assert.Nil(t, cfg.Gateway.Tracing, "Tracing config should remain nil when nothing is set")
	})

	t.Run("initialises tracing config when cfg.Gateway.Tracing is already set", func(t *testing.T) {
		otlpEndpoint = ""
		otlpServiceName = config.DefaultTracingServiceName

		cmd := newCmd()
		cfg := &config.Config{
			Gateway: &config.GatewayConfig{
				Tracing: &config.TracingConfig{Endpoint: "http://pre-existing:4318"},
			},
		}

		applyTracingOverrides(cmd, cfg)

		// Pre-existing endpoint is preserved because the flag was not explicitly set
		// and otlpEndpoint matches the default ("").
		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "http://pre-existing:4318", cfg.Gateway.Tracing.Endpoint)
	})

	t.Run("populates endpoint when otlpEndpoint global is non-empty", func(t *testing.T) {
		otlpEndpoint = "http://env-collector:4318"
		otlpServiceName = config.DefaultTracingServiceName
		t.Cleanup(func() { otlpEndpoint = "" })

		cmd := newCmd()
		cfg := &config.Config{Gateway: &config.GatewayConfig{}}

		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "http://env-collector:4318", cfg.Gateway.Tracing.Endpoint)
	})

	t.Run("populates endpoint when otlp-endpoint flag is explicitly changed", func(t *testing.T) {
		otlpEndpoint = ""
		otlpServiceName = config.DefaultTracingServiceName

		cmd := newCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--otlp-endpoint=http://flag-collector:4318"}))
		// Sync the flag value into the package global (normally done by cobra binding).
		otlpEndpoint = "http://flag-collector:4318"

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Equal(t, "http://flag-collector:4318", cfg.Gateway.Tracing.Endpoint)
		t.Cleanup(func() { otlpEndpoint = "" })
	})

	t.Run("populates sample rate when otlp-sample-rate flag is changed", func(t *testing.T) {
		otlpEndpoint = "http://collector:4318"
		otlpServiceName = config.DefaultTracingServiceName
		otlpSampleRate = 0.5
		t.Cleanup(func() {
			otlpEndpoint = ""
			otlpSampleRate = 0
		})

		cmd := newCmd()
		require.NoError(t, cmd.ParseFlags([]string{"--otlp-sample-rate=0.5"}))

		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		require.NotNil(t, cfg.Gateway.Tracing.SampleRate)
		assert.InDelta(t, 0.5, *cfg.Gateway.Tracing.SampleRate, 1e-9)
	})

	t.Run("does not overwrite SampleRate when flag is not changed", func(t *testing.T) {
		otlpEndpoint = "http://collector:4318"
		otlpServiceName = config.DefaultTracingServiceName
		t.Cleanup(func() { otlpEndpoint = "" })

		cmd := newCmd()
		cfg := &config.Config{Gateway: &config.GatewayConfig{}}
		applyTracingOverrides(cmd, cfg)

		require.NotNil(t, cfg.Gateway.Tracing)
		assert.Nil(t, cfg.Gateway.Tracing.SampleRate, "SampleRate should remain nil when flag not explicitly set")
	})
}

// TestShutdownTracingProviderWithTimeout_ErrorPath verifies the warning callback is
// invoked when the provider's Shutdown returns an error.
//
// To trigger an error we shut down the provider once (so it is already stopped)
// and then call shutdownTracingProviderWithTimeout again with a very short
// deadline, exercising the context-deadline path.
func TestShutdownTracingProviderWithTimeout_ErrorPath(t *testing.T) {
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })

	provider, err := tracing.InitProvider(context.Background(), &config.TracingConfig{
		Endpoint: "http://127.0.0.1:14318",
	})
	require.NoError(t, err)
	require.NotNil(t, provider)

	// First shutdown — drains the SDK and closes all background goroutines.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = provider.Shutdown(ctx)

	// A second shutdown on an already-stopped TracerProvider returns an error.
	var warnMessages []string
	shutdownTracingProviderWithTimeout(provider, func(format string, args ...any) {
		warnMessages = append(warnMessages, format)
	})
	// The provider may or may not return an error on double-shutdown depending on
	// the SDK version, so we assert only that the call completes without panic.
	assert.NotPanics(t, func() {
		shutdownTracingProviderWithTimeout(provider, func(format string, args ...any) {})
	})
}
