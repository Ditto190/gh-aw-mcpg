package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/delegation"
	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/github/gh-aw-mcpg/internal/httputil"
	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/util"
)

var logServerDelegation = logger.ForFile()

type delegatedToolAuthorizationKey struct{}

type delegatedToolAuthorization struct {
	serverID string
	toolName string
}

func (us *UnifiedServer) delegationEnabled() bool {
	return us != nil && us.delegation != nil
}

func (us *UnifiedServer) isDelegatedExecutorAuth(authorizationHeader string) bool {
	if !us.delegationEnabled() {
		return false
	}
	return us.delegation.Store.HasLiveExecutorBearer(authorizationHeader)
}

func (us *UnifiedServer) isDelegatedExecutorSession(sessionID string) bool {
	if !us.delegationEnabled() {
		return false
	}
	return us.delegation.Store.HasLiveExecutorBearer(sessionID)
}

func (us *UnifiedServer) authorizeDelegatedToolCall(ctx context.Context, serverID, toolName string, args interface{}) (context.Context, error) {
	if !us.delegationEnabled() {
		return ctx, nil
	}
	sessionID := us.getSessionID(ctx)
	if !us.delegation.Store.HasLiveExecutorBearer(sessionID) {
		return ctx, nil
	}
	if serverID != "github" {
		return ctx, fmt.Errorf("delegated identity is not authorized for server %q", serverID)
	}
	repository, ok := delegatedToolRepository(toolName, args)
	if !ok {
		return ctx, fmt.Errorf("delegated identity requires canonical owner/repo arguments for tool %q", toolName)
	}
	handle, err := us.delegation.Store.AuthorizeExecutor(sessionID, repository, toolName)
	if err != nil {
		logServerDelegation.Printf("Delegated tool call denied: tool=%s repo_hash=%s", toolName, util.HashForLog(repository, 16, ""))
		return ctx, err
	}
	ctx = guard.SetAgentIDInContext(ctx, "delegation:"+handle)
	ctx = context.WithValue(ctx, delegatedToolAuthorizationKey{}, delegatedToolAuthorization{
		serverID: serverID,
		toolName: toolName,
	})
	return ctx, nil
}

func delegatedToolRepository(toolName string, args interface{}) (string, bool) {
	if !delegation.IsDelegatedTool(toolName) {
		return "", false
	}
	argsMap, ok := args.(map[string]interface{})
	if !ok {
		return "", false
	}
	owner, ownerOK := argsMap["owner"].(string)
	repo, repoOK := argsMap["repo"].(string)
	if !ownerOK || !repoOK {
		return "", false
	}
	repository := owner + "/" + repo
	return repository, delegation.IsCanonicalRepositorySelector(repository)
}

func delegatedToolAuthorized(ctx context.Context, serverID, toolName string) bool {
	authorization, ok := ctx.Value(delegatedToolAuthorizationKey{}).(delegatedToolAuthorization)
	return ok && authorization.serverID == serverID && authorization.toolName == toolName
}

// delegatedAllowedMethods are the only JSON-RPC methods a delegated executor
// bearer may invoke on the /mcp data plane. Everything else, including
// prompts/resources capabilities and unrecognized methods, is denied.
var delegatedAllowedMethods = map[string]bool{
	"initialize":                true,
	"notifications/initialized": true,
	"ping":                      true,
	"tools/list":                true,
	"tools/call":                true,
}

func (us *UnifiedServer) rejectDelegatedNonToolMethods(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !us.isDelegatedExecutorAuth(r.Header.Get("Authorization")) || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		body, err := readAndRestoreRequestBody(r)
		if err != nil {
			logServerDelegation.Printf("Delegated MCP request denied: failed to read body: %v", err)
			httputil.WriteErrorResponse(w, http.StatusForbidden, "delegation_method_denied", "delegated identity is not authorized for this MCP request")
			return
		}
		methods, ok := parseDelegatedRequestMethods(body)
		if !ok || len(methods) == 0 {
			logServerDelegation.Printf("Delegated MCP request denied: unparsable or empty request envelope")
			httputil.WriteErrorResponse(w, http.StatusForbidden, "delegation_method_denied", "delegated identity is not authorized for this MCP request")
			return
		}
		for _, method := range methods {
			if !delegatedAllowedMethods[method] {
				logServerDelegation.Printf("Delegated MCP method denied: method=%s", method)
				httputil.WriteErrorResponse(w, http.StatusForbidden, "delegation_method_denied", "delegated identity is not authorized for this MCP method")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// parseDelegatedRequestMethods extracts every JSON-RPC "method" value from a
// request body, which may be either a single request object or a batch
// array of request objects (accepted by the SDK for protocol versions
// before 2025-06-18). It fails closed: any parse failure, or any batch
// element missing a non-empty method, returns ok=false so the caller denies
// the request instead of passing it through unauthorized.
func parseDelegatedRequestMethods(body []byte) ([]string, bool) {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 {
		return nil, false
	}
	var envelopes []json.RawMessage
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &envelopes); err != nil {
			return nil, false
		}
		if len(envelopes) == 0 {
			return nil, false
		}
	} else {
		envelopes = []json.RawMessage{json.RawMessage(trimmed)}
	}
	methods := make([]string, 0, len(envelopes))
	for _, envelope := range envelopes {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(envelope, &request); err != nil || request.Method == "" {
			return nil, false
		}
		methods = append(methods, request.Method)
	}
	return methods, true
}

// ControlHandler returns the private delegation control-plane handler. It is
// intentionally separate from the MCP data plane so executor bearers cannot
// reach control operations through /mcp.
func (us *UnifiedServer) ControlHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !us.delegationEnabled() || !strings.HasPrefix(r.URL.Path, delegation.ControlPathPrefix) {
			http.NotFound(w, r)
			return
		}
		us.handleDelegationControl(w, r)
	})
}

func (us *UnifiedServer) handleDelegationControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || us.delegation.Capability.Authenticate(r.Header.Get("Authorization")) != nil {
		logServerDelegation.Printf("Delegation control access denied: method=%s path=%s", r.Method, r.URL.Path)
		httputil.WriteErrorResponse(w, http.StatusForbidden, "delegation_access_denied", "delegation control access denied")
		return
	}

	switch r.URL.Path {
	case delegation.ControlPathPrefix + "create-or-confirm":
		var requestWire delegation.CreateOrConfirmRequestWire
		if !decodeServerDelegationJSON(w, r, &requestWire) {
			return
		}
		request, err := requestWire.ToRequest()
		if err != nil {
			httputil.WriteErrorResponse(w, http.StatusBadRequest, "invalid_delegation_request", "invalid delegation request")
			return
		}
		result, err := us.delegation.Store.CreateOrConfirm(request)
		if err != nil {
			if !us.persistDelegationState(w) {
				return
			}
			httputil.WriteErrorResponse(w, http.StatusForbidden, "delegation_request_denied", "delegation request denied")
			return
		}
		if !us.persistDelegationState(w) {
			return
		}
		httputil.WriteJSONResponse(w, http.StatusOK, result)
	case delegation.ControlPathPrefix + "revoke":
		var request struct {
			Handle string `json:"handle"`
		}
		if !decodeServerDelegationJSON(w, r, &request) {
			return
		}
		if err := us.delegation.Store.Revoke(request.Handle); err != nil {
			httputil.WriteErrorResponse(w, http.StatusInternalServerError, "delegation_revoke_failed", "delegation revoke failed")
			return
		}
		if !us.persistDelegationState(w) {
			return
		}
		httputil.WriteJSONResponse(w, http.StatusOK, map[string]bool{"revoked": true})
	case delegation.ControlPathPrefix + "revoke-by-labels":
		var request struct {
			RunID          string `json:"run_id"`
			EnclaveEntryID string `json:"enclave_entry_id"`
		}
		if !decodeServerDelegationJSON(w, r, &request) {
			return
		}
		revoked := us.delegation.Store.RevokeByLabels(request.RunID, request.EnclaveEntryID)
		if !us.persistDelegationState(w) {
			return
		}
		httputil.WriteJSONResponse(w, http.StatusOK, map[string]int{"revoked": revoked})
	case delegation.ControlPathPrefix + "status":
		var request struct {
			RunID          string `json:"run_id"`
			EnclaveEntryID string `json:"enclave_entry_id"`
		}
		if !decodeServerDelegationJSON(w, r, &request) {
			return
		}
		if request.RunID == "" || request.EnclaveEntryID == "" {
			httputil.WriteErrorResponse(w, http.StatusBadRequest, "delegation_status_invalid_request", "run_id and enclave_entry_id are required")
			return
		}
		status := us.delegation.Store.Status()
		httputil.WriteJSONResponse(w, http.StatusOK, map[string]any{
			"recovery_incomplete": status.RecoveryIncomplete,
			"generation":          status.Generation,
			"live_identity_count": status.LiveIdentityCount,
			"labelled_handles":    us.delegation.Store.LabelHandles(request.RunID, request.EnclaveEntryID),
		})
	case delegation.ControlPathPrefix + "reconcile":
		var request struct{}
		if !decodeServerDelegationJSON(w, r, &request) {
			return
		}
		if err := us.delegation.Store.MarkReconciledAndSaveState(us.delegation.StatePath); err != nil {
			httputil.WriteErrorResponse(w, http.StatusInternalServerError, "delegation_state_persist_failed", "delegation state persistence failed")
			return
		}
		httputil.WriteJSONResponse(w, http.StatusOK, map[string]bool{"reconciled": true})
	default:
		http.NotFound(w, r)
	}
}

func (us *UnifiedServer) persistDelegationState(w http.ResponseWriter) bool {
	if err := us.delegation.Store.SaveState(us.delegation.StatePath); err != nil {
		httputil.WriteErrorResponse(w, http.StatusInternalServerError, "delegation_state_persist_failed", "delegation state persistence failed")
		return false
	}
	return true
}

func decodeServerDelegationJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		httputil.WriteErrorResponse(w, http.StatusBadRequest, "invalid_delegation_request", "invalid delegation request")
		return false
	}
	return true
}
