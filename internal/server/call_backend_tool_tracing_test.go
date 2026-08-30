package server

import (
	"context"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/tracing"
	"github.com/github/gh-aw-mcpg/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestCallBackendTool_ToolCallSpanHasConversationID verifies that the mcp.tool_call span
// started by callBackendTool carries gen_ai.conversation.id set to the redacted, stable
// session attribution (HashIdentifierForLog) so that session-scoped queries work on child
// spans independently of the parent gateway.request span without exposing the raw identity.
func TestCallBackendTool_ToolCallSpanHasConversationID(t *testing.T) {
	const sessionID = "sess-abc"

	backend := newBackendWithToolResponse(t, "list_issues", defaultToolResponse)
	defer backend.Close()

	us := makeUnifiedWithGuard(t, "tracing-conversation-id-guard", &difcTestGuard{}, backend, "filter")

	// Inject a recording tracer into the unified server so mcp.tool_call spans are captured.
	exporter := tracetest.NewInMemoryExporter()
	sp := sdktrace.NewSimpleSpanProcessor(exporter)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	us.CachedTracer = tracing.CachedTracer{Tracer: tp.Tracer("test")}

	result, _, err := us.callBackendTool(callCtx(sessionID), "test-server", "list_issues", map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, result)

	spans := exporter.GetSpans()

	// Locate the mcp.tool_call span.
	var toolSpan *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "mcp.tool_call" {
			toolSpan = &spans[i]
			break
		}
	}
	require.NotNil(t, toolSpan, "mcp.tool_call span must be recorded")

	// The span must carry gen_ai.conversation.id set to the redacted session attribution.
	wantConversationID := util.HashIdentifierForLog(sessionID)
	var gotConversationID string
	for _, attr := range toolSpan.Attributes {
		if attr.Key == tracing.GenAIConversationID {
			gotConversationID = attr.Value.AsString()
			break
		}
	}
	assert.Equal(t, wantConversationID, gotConversationID,
		"mcp.tool_call span must carry gen_ai.conversation.id set to the formatted session ID")
}
