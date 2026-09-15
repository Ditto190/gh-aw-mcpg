package server

import (
	"context"

	"github.com/github/gh-aw-mcpg/internal/mcp"
	"github.com/github/gh-aw-mcpg/internal/util"
)

// isEnclaveSession reports whether sessionID belongs to an enclave-scoped identity.
//
// Two identities are enclave-scoped: an agent whose per-agent policy sets
// enclave = true, and a delegated executor bearer (delegation admits enclaves
// dynamically). Both are authorized to disclose only a bounded, broker-validated
// result, so their MCP payloads must never be persisted to a log sink that a
// workflow outside the enclave exports as an artifact.
func (us *UnifiedServer) isEnclaveSession(sessionID string) bool {
	if us == nil {
		return false
	}
	if us.isDelegatedExecutorSession(sessionID) {
		return true
	}
	return us.cfg.AgentPolicyFor(sessionID).IsEnclave()
}

// markEnclaveSessionContext attaches the enclave provenance marker to ctx when
// sessionID is enclave-scoped. The marker travels with the request through tool
// dispatch, guard labeling, and every nested backend call.
func (us *UnifiedServer) markEnclaveSessionContext(ctx context.Context, sessionID string) context.Context {
	if !us.isEnclaveSession(sessionID) {
		return ctx
	}
	logSession.Printf("Marking session as enclave-scoped: sessionID=%s", util.HashIdentifierForLog(sessionID))
	return mcp.WithEnclaveSession(ctx)
}
