package tracing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/github/gh-aw-mcpg/internal/tracing"
)

func TestCachedTracer_GetTracer_ReturnsCached(t *testing.T) {
	cached := noop.NewTracerProvider().Tracer("cached")
	holder := tracing.CachedTracer{Tracer: cached}
	assert.Equal(t, cached, holder.GetTracer())
}

func TestCachedTracer_GetTracer_WithNilCachedTracer_ReturnsGlobal(t *testing.T) {
	ctx := context.Background()
	provider, err := tracing.InitProvider(ctx, nil)
	require.NoError(t, err)
	defer provider.Shutdown(ctx)

	holder := tracing.CachedTracer{}
	assert.NotNil(t, holder.GetTracer())
}
