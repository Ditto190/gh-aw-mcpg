// Package tracing provides OpenTelemetry OTLP trace export for the MCP Gateway.
// This file holds canary tests for go.opentelemetry.io/otel* upgrades. The gateway
// implements sdktrace.SpanExporter itself (fanout.go) and relies on the SDK tracer
// provider flushing buffered spans on Shutdown, so both semantics must be verified
// whenever the otel module family is bumped in go.mod.
package tracing

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestSpanExporterInterfaceCanary is a canary test for otel SDK upgrades.
//
// fanoutExporter (fanout.go) hand-implements sdktrace.SpanExporter so that spans can
// be fanned out to multiple OTLP backends. If the SDK adds, removes, or reshapes a
// method on that interface, the gateway's fan-out exporter would either stop
// compiling or silently lose behaviour behind a new default method.
//
// Upgrade gate: this canary and TestTracerProviderShutdownFlushesCanary must both
// pass before accepting a go.opentelemetry.io/otel* upgrade, alongside the semconv
// lockstep check in TestSchemaURL (semconv_test.go).
func TestSpanExporterInterfaceCanary(t *testing.T) {
	assert := assert.New(t)

	exporterType := reflect.TypeOf((*sdktrace.SpanExporter)(nil)).Elem()
	require.Equal(t, reflect.Interface, exporterType.Kind())

	// Method set must stay exactly ExportSpans + Shutdown (alphabetical order).
	wantSignatures := map[string]string{
		"ExportSpans": "func(context.Context, []trace.ReadOnlySpan) error",
		"Shutdown":    "func(context.Context) error",
	}
	assert.Len(wantSignatures, exporterType.NumMethod(),
		"sdktrace.SpanExporter method set changed; review fanoutExporter in fanout.go")

	for i := range exporterType.NumMethod() {
		method := exporterType.Method(i)
		want, ok := wantSignatures[method.Name]
		assert.True(ok, "unexpected SpanExporter method %q; review fanoutExporter in fanout.go", method.Name)
		if ok {
			assert.Equal(want, method.Type.String(),
				"SpanExporter.%s signature changed; review fanoutExporter in fanout.go", method.Name)
		}
	}

	// The gateway's own exporter must still satisfy the interface.
	var _ sdktrace.SpanExporter = (*fanoutExporter)(nil)
}

// TestTracerProviderShutdownFlushesCanary is a canary test for otel SDK upgrades.
//
// Provider.Shutdown (provider.go) calls only sdktrace.TracerProvider.Shutdown and
// relies on it flushing spans still buffered in the batch span processor; the
// gateway never calls ForceFlush separately. It also relies on Shutdown being safe
// to call more than once (the CLI defers a shutdown that may race an explicit one).
//
// If the SDK stops flushing on Shutdown, buffered gateway spans would be dropped at
// exit without any error surfacing.
func TestTracerProviderShutdownFlushesCanary(t *testing.T) {
	assert := assert.New(t)

	// Record span names eagerly: the batch span processor reuses (and clears) the
	// slice it hands to the exporter, and tracetest.InMemoryExporter discards its
	// recorded spans on Shutdown, so neither can be inspected after Shutdown.
	exporter := &nameRecordingExporter{}
	tp := sdktrace.NewTracerProvider(
		// A long batch timeout ensures the span is only exported because of Shutdown.
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Hour)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	_, span := tp.Tracer(instrumentationName).Start(context.Background(), "canary-span")
	span.End()
	assert.Empty(exporter.names(), "span should still be buffered before Shutdown")

	require.NoError(t, tp.Shutdown(context.Background()),
		"TracerProvider.Shutdown should succeed")
	assert.Equal([]string{"canary-span"}, exporter.names(),
		"TracerProvider.Shutdown must flush buffered spans; review Provider.Shutdown in provider.go")

	// Shutdown must remain idempotent for the gateway's deferred shutdown path.
	assert.NoError(tp.Shutdown(context.Background()),
		"repeated TracerProvider.Shutdown must not error")
}

// nameRecordingExporter is a SpanExporter that records exported span names.
type nameRecordingExporter struct {
	mu        sync.Mutex
	spanNames []string
}

func (e *nameRecordingExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, span := range spans {
		e.spanNames = append(e.spanNames, span.Name())
	}
	return nil
}

func (e *nameRecordingExporter) Shutdown(context.Context) error { return nil }

// names returns a copy of the recorded span names.
func (e *nameRecordingExporter) names() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.spanNames...)
}
