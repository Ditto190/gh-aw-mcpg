// Package tracing provides OpenTelemetry OTLP trace export for the MCP Gateway.
// This file tests the semconv.go re-export helpers.
package tracing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

// TestErrorType verifies that ErrorType wraps the semconv error.type attribute
// with the correct key and value derived from the error's concrete type name.
func TestErrorType(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantKey   string
		wantValue string
	}{
		{
			name:      "stdlib errors.New",
			err:       errors.New("boom"),
			wantKey:   "error.type",
			wantValue: "*errors.errorString",
		},
		{
			name:      "errors.Join wrapping",
			err:       errors.Join(errors.New("wrapped")),
			wantKey:   "error.type",
			wantValue: "*errors.joinError",
		},
		{
			name:      "custom error type",
			err:       &customError{msg: "custom"},
			wantKey:   "error.type",
			wantValue: "*tracing.customError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := ErrorType(tt.err)
			assert.Equal(t, tt.wantKey, string(kv.Key))
			assert.Equal(t, tt.wantValue, kv.Value.AsString())
		})
	}
}

// TestErrorType_KeyMatchesErrorTypeKey verifies that the Key returned by ErrorType
// is exactly ErrorTypeKey, so callers can use hasAttr with the same constant.
func TestErrorType_KeyMatchesErrorTypeKey(t *testing.T) {
	kv := ErrorType(errors.New("test"))
	assert.Equal(t, ErrorTypeKey, kv.Key,
		"ErrorType key must equal the exported ErrorTypeKey constant")
}

// TestServiceName verifies that ServiceName wraps the semconv service.name attribute
// with the correct key and the provided value.
func TestServiceName(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "typical service name", input: "mcp-gateway"},
		{name: "empty string", input: ""},
		{name: "name with spaces", input: "my service"},
		{name: "name with special chars", input: "svc-v2.0/prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := ServiceName(tt.input)
			assert.Equal(t, "service.name", string(kv.Key))
			assert.Equal(t, tt.input, kv.Value.AsString())
		})
	}
}

// TestServiceVersion verifies that ServiceVersion wraps the semconv service.version
// attribute with the correct key and the provided value.
func TestServiceVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "semver", input: "1.2.3"},
		{name: "empty string", input: ""},
		{name: "commit hash", input: "abc1234"},
		{name: "release label", input: "v0.1.0-beta.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kv := ServiceVersion(tt.input)
			assert.Equal(t, "service.version", string(kv.Key))
			assert.Equal(t, tt.input, kv.Value.AsString())
		})
	}
}

// TestSemconvConstantsHaveExpectedKeys verifies that the re-exported semconv
// constants carry the correct wire-format attribute key strings.
func TestSemconvConstantsHaveExpectedKeys(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantKey string
	}{
		{"HTTPRequestMethodKey", string(HTTPRequestMethodKey), "http.request.method"},
		{"HTTPRouteKey", string(HTTPRouteKey), "http.route"},
		{"HTTPResponseStatusCodeKey", string(HTTPResponseStatusCodeKey), "http.response.status_code"},
		{"URLPathKey", string(URLPathKey), "url.path"},
		{"ServerAddressKey", string(ServerAddressKey), "server.address"},
		{"ErrorTypeKey", string(ErrorTypeKey), "error.type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantKey, tt.key,
				"constant %s must match the semconv wire-format key", tt.name)
		})
	}
}

// TestGenAIConstantsHaveExpectedKeys verifies that the GenAI attribute keys and
// the custom MCP keys carry the correct wire-format strings.
func TestGenAIConstantsHaveExpectedKeys(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantKey string
	}{
		{"GenAIToolName", string(GenAIToolName), "gen_ai.tool.name"},
		{"GenAIOperationName", string(GenAIOperationName), "gen_ai.operation.name"},
		{"GenAIConversationID", string(GenAIConversationID), "gen_ai.conversation.id"},
		{"GenAIAgentName", string(GenAIAgentName), "gen_ai.agent.name"},
		{"GenAIAgentID", string(GenAIAgentID), "gen_ai.agent.id"},
		// Custom MCP attribute keys from genai_attrs.go
		{"GenAISystem", string(GenAISystem), "gen_ai.system"},
		{"MCPMethod", string(MCPMethod), "mcp.method"},
		{"MCPResponseStatus", string(MCPResponseStatus), "mcp.response.status"},
		{"RateLimitHit", string(RateLimitHit), "rate_limit.hit"},
		{"GatewayTag", string(GatewayTag), "gateway.tag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantKey, tt.key,
				"constant %s must match the expected wire-format key", tt.name)
		})
	}
}

// TestServiceName_UsedInSpanAttribute verifies that ServiceName and ServiceVersion
// can be passed to a span as attributes without error.
func TestServiceName_UsedInSpanAttribute(t *testing.T) {
	span, getSpans := newRecordingSpan(t, "service-name-test")

	span.SetAttributes(ServiceName("test-svc"), ServiceVersion("1.0.0"))

	spans := getSpans()
	require.Len(t, spans, 1)
	assert.True(t, hasAttr(spans[0].Attributes, attribute.Key("service.name"), "test-svc"),
		"span must carry the service.name attribute")
	assert.True(t, hasAttr(spans[0].Attributes, attribute.Key("service.version"), "1.0.0"),
		"span must carry the service.version attribute")
}

// expectedSchemaURL is the semconv schema URL matching the semconv version imported by
// semconv.go. Update it together with the semconv import path when bumping the otel modules.
const expectedSchemaURL = "https://opentelemetry.io/schemas/1.43.0"

// TestSchemaURL_MatchesPinnedSemconvVersion guards against semconv version drift: the
// re-exported SchemaURL must stay in lockstep with the semconv version used by the pinned
// otel/sdk in go.mod, otherwise resource detection fails with a "conflicting Schema URL"
// error. The SDK's own schema URL is read from a detected resource so that bumping only
// otel/sdk (without updating semconv.go) fails here instead of at runtime.
func TestSchemaURL_MatchesPinnedSemconvVersion(t *testing.T) {
	sdkResource, err := resource.New(context.Background(), resource.WithTelemetrySDK())
	require.NoError(t, err)
	assert.Equal(t, sdkResource.SchemaURL(), SchemaURL,
		"otel/sdk semconv version changed; update the semconv import in semconv.go to match")
	assert.Equal(t, expectedSchemaURL, SchemaURL,
		"semconv version changed; update semconv.go imports and expectedSchemaURL together")

	serviceResource := resource.NewWithAttributes(
		SchemaURL,
		ServiceName("mcp-gateway"),
		ServiceVersion("1.0.0"),
	)
	mergedResource, err := resource.Merge(sdkResource, serviceResource)
	require.NoError(t, err, "SDK and service resources must use the same schema URL")
	assert.Equal(t, SchemaURL, mergedResource.SchemaURL(),
		"merged resource must preserve the shared schema URL")
}

// customError is a local error type used to test ErrorType with a non-stdlib error.
type customError struct {
	msg string
}

func (e *customError) Error() string { return e.msg }
