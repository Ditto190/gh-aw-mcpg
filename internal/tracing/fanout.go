package tracing

import (
	"context"
	"errors"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// fanoutExporter is a SpanExporter that fans out span export to multiple
// underlying exporters. All exporters are attempted even when earlier ones
// fail (partial-failure tolerance), and collected errors are joined before
// returning.
type fanoutExporter struct {
	exporters []sdktrace.SpanExporter
}

// newFanoutExporter returns a SpanExporter that forwards to all given exporters.
// When only one exporter is provided it is returned directly to avoid overhead.
func newFanoutExporter(exporters []sdktrace.SpanExporter) sdktrace.SpanExporter {
	if len(exporters) == 1 {
		return exporters[0]
	}
	return &fanoutExporter{exporters: exporters}
}

// ExportSpans exports spans to each underlying exporter in order. Export
// continues to the next exporter even if the current one returns an error so
// that a single backend failure does not prevent delivery to the others. All
// errors are joined and returned.
func (f *fanoutExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	var errs []error
	for _, exp := range f.exporters {
		if err := exp.ExportSpans(ctx, spans); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Shutdown shuts down each underlying exporter in order, collecting any
// errors. All errors are joined and returned.
func (f *fanoutExporter) Shutdown(ctx context.Context) error {
	var errs []error
	for _, exp := range f.exporters {
		if err := exp.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
