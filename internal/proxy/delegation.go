package proxy

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/github/gh-aw-mcpg/internal/delegation"
	"github.com/github/gh-aw-mcpg/internal/enclavegithub"
	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/util"
)

var logDelegation = logger.ForFile()

const delegationControlPath = delegation.ControlPathPrefix

type delegationState struct {
	store      *delegation.Store
	capability *delegation.ControlCapability
	statePath  string
}

// DelegationConfig enables runtime repository-read delegation and its
// AWF-authenticated private control channel.
type DelegationConfig = delegation.RuntimeConfig

func newDelegationState(cfg *DelegationConfig) (*delegationState, error) {
	if cfg == nil {
		return nil, nil
	}
	if cfg.Store == nil || cfg.Capability == nil || cfg.StatePath == "" {
		return nil, fmt.Errorf("delegation store, control capability, and state path are required")
	}
	return &delegationState{store: cfg.Store, capability: cfg.Capability, statePath: cfg.StatePath}, nil
}

func (h *proxyHandler) handleDelegationControl(w http.ResponseWriter, r *http.Request) {
	delegation.HandleControl(w, r, delegation.ControlDeps{
		Store:      h.server.delegation.store,
		Capability: h.server.delegation.capability,
		StatePath:  h.server.delegation.statePath,
	}, logDelegation.Printf)
}

// ControlHandler returns the private control-plane handler. It is intentionally
// separate from Handler so executor-facing GitHub traffic cannot reach control
// operations even if it presents a valid executor bearer.
func (s *Server) ControlHandler() http.Handler {
	var deps delegation.ControlDeps
	if s != nil && s.delegation != nil {
		deps = delegation.ControlDeps{
			Store:      s.delegation.store,
			Capability: s.delegation.capability,
			StatePath:  s.delegation.statePath,
		}
	}
	return delegation.NewControlHTTPHandler(deps, logDelegation.Printf)
}

func (h *proxyHandler) handleDelegatedRequest(w http.ResponseWriter, r *http.Request) {
	path, ok := enclavePath(r.URL.Path, r.URL.RawPath)
	if !ok || r.Method != http.MethodGet || hasEnclaveGETBody(r) {
		writeEnclaveDenied(w)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeEnclaveDenied(w)
		return
	}
	route, err := enclavegithub.MatchRoute(path, query)
	if err != nil {
		logDelegation.Printf("No matching enclave route for path_hash=%s", util.HashForLog(path, 16, ""))
		writeEnclaveDenied(w)
		return
	}
	toolName, args := enclaveToolAndArgs(route)
	if toolName == "" {
		writeEnclaveDenied(w)
		return
	}
	handle, err := h.server.delegation.store.AuthorizeExecutor(r.Header.Get("Authorization"), route.FullRepo(), toolName)
	if err != nil {
		logDelegation.Printf("Executor not authorized for tool=%s repo_hash=%s", toolName, util.HashForLog(route.FullRepo(), 16, ""))
		writeEnclaveDenied(w)
		return
	}
	fullPath := path
	if r.URL.RawQuery != "" {
		fullPath += "?" + r.URL.RawQuery
	}
	logDelegation.Printf("Delegating request: tool=%s repo_hash=%s path_hash=%s", toolName, util.HashForLog(route.FullRepo(), 16, ""), util.HashForLog(path, 16, ""))
	// Bind this request to a delegation-specific isolation context, keyed
	// on the identity's own opaque handle and assigned repository, rather
	// than letting it fall through to the shared fallback proxy DIFC
	// identity used by ordinary (non-delegated, non-enclave) requests.
	ctx := withEnclaveAuthorization(r.Context(), "delegation:"+handle, route.FullRepo())
	h.handleWithDIFC(w, r.WithContext(ctx), fullPath, toolName, args, nil)
}
