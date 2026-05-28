package tracing

import (
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// RecordSpanError records an error with stack trace and sets span status to Error.
func RecordSpanError(span oteltrace.Span, err error, msg string) {
	span.RecordError(err, oteltrace.WithStackTrace(true))
	span.SetStatus(codes.Error, msg)
}
