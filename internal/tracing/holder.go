package tracing

import "go.opentelemetry.io/otel/trace"

// CachedTracer holds an optional pre-initialized tracer and falls back to the
// global tracer when the cached tracer is nil.
type CachedTracer struct {
	Tracer trace.Tracer
}

// GetTracer returns the cached tracer when set, otherwise the global tracer.
func (ct CachedTracer) GetTracer() trace.Tracer {
	return GetCachedOrGlobal(ct.Tracer)
}
