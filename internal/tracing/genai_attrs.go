// Package tracing provides OpenTelemetry OTLP trace export for the MCP Gateway.
// This file defines custom and non-semconv attribute keys for gen_ai and MCP spans.
// The GenAI attribute keys (GenAIToolName, GenAIOperationName, etc.) are
// defined in semconv.go alongside the semconv re-exports.
package tracing

import "go.opentelemetry.io/otel/attribute"

// GenAISystem identifies the GenAI system family for MCP spans.
// The key string is preserved for wire compatibility with observability backends
// that still expect it.
const GenAISystem = attribute.Key("gen_ai.system")

// MCP-specific attribute keys (no gen_ai equivalent in the spec).
const (
	// MCPMethod is the JSON-RPC method name (e.g. "tools/call").
	MCPMethod = attribute.Key("mcp.method")

	// MCPResponseStatus is the conceptual HTTP status of the proxied MCP response.
	// Used on internal MCP spans (e.g. mcp.tool_call) instead of the HTTP-specific
	// semconv.HTTPResponseStatusCodeKey, which is semantically reserved for HTTP spans.
	MCPResponseStatus = attribute.Key("mcp.response.status")

	// RateLimitHit indicates a rate limit was triggered.
	RateLimitHit = attribute.Key("rate_limit.hit")

	// GatewayTag is the routing mode tag (unified/routed).
	GatewayTag = attribute.Key("gateway.tag")
)
