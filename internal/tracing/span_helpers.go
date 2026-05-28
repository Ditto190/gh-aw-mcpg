// Package tracing provides OpenTelemetry OTLP trace export for the MCP Gateway.
// This file provides span error recording helpers.
package tracing

import (
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// RecordSpanError records err on span with a stack trace and sets the span status to Error.
// Use this instead of calling RecordError + SetStatus individually to ensure consistent
// behavior (stack traces enabled, status always set) across all error paths.
func RecordSpanError(span oteltrace.Span, err error, msg string) {
	span.RecordError(err, oteltrace.WithStackTrace(true))
	span.SetStatus(codes.Error, msg)
}

// RecordSpanErrorOnAll records err on all provided spans with a stack trace and sets their
// status to Error. Useful when both a parent and child span must reflect the same failure.
func RecordSpanErrorOnAll(err error, msg string, spans ...oteltrace.Span) {
	for _, span := range spans {
		RecordSpanError(span, err, msg)
	}
}
