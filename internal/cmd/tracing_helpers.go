package cmd

import (
	"context"
	"time"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/envutil"
	"github.com/github/gh-aw-mcpg/internal/tracing"
	"github.com/spf13/pflag"
)

func registerTracingFlags(flags *pflag.FlagSet, endpoint *string, serviceName *string, sampleRate *float64, endpointUsage string, serviceUsage string, sampleUsage string) {
	flags.StringVar(endpoint, "otlp-endpoint", envutil.GetEnvString("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		endpointUsage)
	flags.StringVar(serviceName, "otlp-service-name", envutil.GetEnvString("OTEL_SERVICE_NAME", config.DefaultTracingServiceName),
		serviceUsage)
	flags.Float64Var(sampleRate, "otlp-sample-rate", config.DefaultTracingSampleRate,
		sampleUsage)
}

func initTracingProviderWithFallback(
	ctx context.Context,
	tracingCfg *config.TracingConfig,
	initWarningFormat string,
	warnf func(format string, args ...any),
) *tracing.Provider {
	debugLog.Print("Initializing tracing provider")
	tracingProvider, err := tracing.InitProvider(ctx, tracingCfg)
	if err != nil {
		debugLog.Printf("Tracing provider init failed, falling back to no-op provider: %v", err)
		warnf(initWarningFormat, err)
		tracingProvider, _ = tracing.InitProvider(ctx, nil)
	} else {
		debugLog.Print("Tracing provider initialized successfully")
	}

	return tracingProvider
}

func shutdownTracingProviderWithTimeout(tracingProvider *tracing.Provider, warnf func(format string, args ...any)) {
	debugLog.Print("Shutting down tracing provider")
	shutdownCtxTracing, cancelTracing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelTracing()

	if err := tracingProvider.Shutdown(shutdownCtxTracing); err != nil {
		debugLog.Printf("Tracing provider shutdown error: %v", err)
		warnf("tracing provider shutdown error: %v", err)
	} else {
		debugLog.Print("Tracing provider shut down successfully")
	}
}
