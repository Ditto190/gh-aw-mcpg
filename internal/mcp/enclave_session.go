package mcp

import (
	"context"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/sanitize"
)

var logEnclaveSession = logger.ForFile()

// EnclaveSessionContextKey marks a request context as belonging to an enclave-scoped
// session. The marker is attached once, when the session is established, and travels
// with the context through tool dispatch, guard labeling, and every nested backend
// call, so no logging site can lose the enclave provenance of the traffic it records.
const EnclaveSessionContextKey ContextKey = "awmg-enclave-session"

// WithEnclaveSession returns a context marked as enclave-scoped.
func WithEnclaveSession(ctx context.Context) context.Context {
	logEnclaveSession.Print("Marking context as enclave-scoped session")
	return context.WithValue(ctx, EnclaveSessionContextKey, true)
}

// IsEnclaveSession reports whether ctx carries the enclave provenance marker.
// Payloads for such requests must never be persisted to an exported log sink.
func IsEnclaveSession(ctx context.Context) bool {
	if ctx == nil {
		logEnclaveSession.Print("IsEnclaveSession: nil context, treating as non-enclave")
		return false
	}
	enclave, _ := ctx.Value(EnclaveSessionContextKey).(bool)
	return enclave
}

// RedactRequestValueForLog returns value unchanged for ordinary traffic and a
// stable keyed digest for enclave-scoped traffic. Method-specific debug logs
// (e.g. the resources/read URI) carry request arguments just like a tools/call
// payload does, so they resolve the same redaction decision from the request
// context instead of bypassing it.
func RedactRequestValueForLog(ctx context.Context, value string) string {
	enclave := IsEnclaveSession(ctx)
	if sanitize.ShouldRedactPayload(enclave) {
		logEnclaveSession.Printf("Redacting request value for log: enclave=%v, valueLen=%d", enclave, len(value))
		return sanitize.KeyedDigest(value)
	}
	logEnclaveSession.Printf("Passing through request value for log unredacted: enclave=%v", enclave)
	return value
}
