package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logHandler = logger.New("proxy:handler")

// proxyHandler implements http.Handler and runs the DIFC pipeline on proxied requests.
type proxyHandler struct {
	server *Server
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Strip the /api/v3 prefix that GH_HOST adds
	path := StripGHHostPrefix(r.URL.Path)

	logHandler.Printf("incoming %s %s", r.Method, path)

	// Health check endpoint
	if path == "/health" || path == "/healthz" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Only filter read operations (GET + GraphQL POST to /graphql)
	isGraphQL := IsGraphQLPath(path)
	isRead := r.Method == http.MethodGet || (r.Method == http.MethodPost && isGraphQL)
	if !isRead {
		// Pass through write operations unmodified
		h.passthrough(w, r, path)
		return
	}

	// Route the request to a guard tool name
	var toolName string
	var args map[string]interface{}
	var graphQLBody []byte

	if isGraphQL {
		// Read and parse the GraphQL body
		var err error
		graphQLBody, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		match := MatchGraphQL(graphQLBody)
		if match == nil {
			// Unknown GraphQL query — pass through with conservative labeling
			h.forwardGraphQL(w, r, path, graphQLBody)
			return
		}
		toolName = match.ToolName
		args = match.Args
	} else {
		match := MatchRoute(path)
		if match == nil {
			// Unknown REST endpoint — pass through
			h.passthrough(w, r, path)
			return
		}
		toolName = match.ToolName
		args = match.Args
	}

	// Run the DIFC pipeline
	h.handleWithDIFC(w, r, path, toolName, args, graphQLBody)
}

// handleWithDIFC runs the 6-phase DIFC pipeline on a request.
func (h *proxyHandler) handleWithDIFC(w http.ResponseWriter, r *http.Request, path, toolName string, args map[string]interface{}, graphQLBody []byte) {
	ctx := r.Context()
	s := h.server
	backend := &stubBackendCaller{}

	if !s.guardInitialized {
		log.Printf("[proxy] WARNING: guard not initialized, passing through")
		if graphQLBody != nil {
			h.forwardGraphQL(w, r, path, graphQLBody)
		} else {
			h.passthrough(w, r, path)
		}
		return
	}

	// **Phase 0: Get agent labels**
	agentLabels := s.agentRegistry.GetOrCreate("proxy")
	logHandler.Printf("[DIFC] Phase 0: agent secrecy=%v integrity=%v",
		agentLabels.GetSecrecyTags(), agentLabels.GetIntegrityTags())

	// **Phase 1: Guard labels the resource**
	resource, operation, err := s.guard.LabelResource(ctx, toolName, args, backend, s.capabilities)
	if err != nil {
		logHandler.Printf("[DIFC] Phase 1 failed: %v", err)
		// On labeling failure, pass through (fail-open for read operations)
		if graphQLBody != nil {
			h.forwardGraphQL(w, r, path, graphQLBody)
		} else {
			h.passthrough(w, r, path)
		}
		return
	}

	logHandler.Printf("[DIFC] Phase 1: resource=%s op=%s secrecy=%v integrity=%v",
		resource.Description, operation,
		resource.Secrecy.Label.GetTags(), resource.Integrity.Label.GetTags())

	// **Phase 2: Coarse-grained access check**
	evalResult := s.evaluator.Evaluate(agentLabels.Secrecy, agentLabels.Integrity, resource, operation)

	if !evalResult.IsAllowed() {
		if operation == difc.OperationRead {
			// Read in filter mode: skip coarse block, proceed to fine-grained filtering
			logHandler.Printf("[DIFC] Phase 2: coarse check failed for read, proceeding to Phase 3")
		} else {
			// Write blocked
			logHandler.Printf("[DIFC] Phase 2: BLOCKED %s %s — %s", r.Method, path, evalResult.Reason)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"message": fmt.Sprintf("DIFC policy violation: %s", evalResult.Reason),
			})
			return
		}
	}

	// **Phase 3: Forward to upstream GitHub API**
	var resp *http.Response
	if graphQLBody != nil {
		resp, err = s.forwardToGitHub(ctx, http.MethodPost, "/graphql", bytes.NewReader(graphQLBody), "application/json")
	} else {
		resp, err = s.forwardToGitHub(ctx, r.Method, path, nil, "")
	}
	if err != nil {
		logHandler.Printf("[DIFC] Phase 3 failed: %v", err)
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusBadGateway)
		return
	}

	// For non-200 responses, pass through as-is
	if resp.StatusCode >= 300 {
		h.writeResponse(w, resp, respBody)
		return
	}

	// Parse the response as JSON for DIFC filtering
	var responseData interface{}
	if err := json.Unmarshal(respBody, &responseData); err != nil {
		// Non-JSON response — pass through
		logHandler.Printf("[DIFC] response is not JSON, passing through")
		h.writeResponse(w, resp, respBody)
		return
	}

	// **Phase 4: Guard labels the response**
	labeledData, err := s.guard.LabelResponse(ctx, toolName, responseData, backend, s.capabilities)
	if err != nil {
		logHandler.Printf("[DIFC] Phase 4 failed: %v", err)
		// On labeling failure, use coarse-grained result
		if evalResult.IsAllowed() {
			h.writeResponse(w, resp, respBody)
		} else {
			h.writeEmptyResponse(w, resp)
		}
		return
	}

	// **Phase 5: Fine-grained filtering**
	var finalData interface{}
	if labeledData != nil {
		if collection, ok := labeledData.(*difc.CollectionLabeledData); ok {
			filtered := s.evaluator.FilterCollection(
				agentLabels.Secrecy, agentLabels.Integrity, collection, operation)

			logHandler.Printf("[DIFC] Phase 5: %d/%d items accessible",
				filtered.GetAccessibleCount(), filtered.TotalCount)

			// Log filtered items
			if filtered.GetFilteredCount() > 0 {
				logHandler.Printf("[DIFC] Filtered %d items", filtered.GetFilteredCount())
				logger.LogInfo("proxy", "DIFC filtered %d/%d items for %s %s (tool=%s)",
					filtered.GetFilteredCount(), filtered.TotalCount, r.Method, path, toolName)
			}

			// Strict mode: block entire response if any item filtered
			if s.enforcementMode == difc.EnforcementStrict && filtered.GetFilteredCount() > 0 {
				logHandler.Printf("[DIFC] STRICT: blocking response — %d filtered items", filtered.GetFilteredCount())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"message": fmt.Sprintf("DIFC policy violation: %d of %d items not accessible",
						filtered.GetFilteredCount(), filtered.TotalCount),
				})
				return
			}

			finalData, err = filtered.ToResult()
			if err != nil {
				logHandler.Printf("[DIFC] Phase 5 ToResult failed: %v", err)
				h.writeEmptyResponse(w, resp)
				return
			}
		} else {
			// Simple labeled data — already passed coarse check
			finalData, err = labeledData.ToResult()
			if err != nil {
				logHandler.Printf("[DIFC] Phase 5 ToResult failed: %v", err)
				h.writeEmptyResponse(w, resp)
				return
			}
		}
	} else {
		// No fine-grained labels — use coarse result
		if evalResult.IsAllowed() {
			finalData = responseData
		} else {
			h.writeEmptyResponse(w, resp)
			return
		}
	}

	// **Phase 6: Label accumulation (propagate mode)**
	if s.enforcementMode == difc.EnforcementPropagate && labeledData != nil {
		overall := labeledData.Overall()
		agentLabels.AccumulateFromRead(overall)
		logHandler.Printf("[DIFC] Phase 6: accumulated labels")
	}

	// Write the filtered response
	filteredJSON, err := json.Marshal(finalData)
	if err != nil {
		http.Error(w, "failed to serialize filtered response", http.StatusInternalServerError)
		return
	}

	// Copy response headers
	copyResponseHeaders(w, resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(filteredJSON)
}

// passthrough forwards a request to the upstream GitHub API without DIFC filtering.
func (h *proxyHandler) passthrough(w http.ResponseWriter, r *http.Request, path string) {
	logHandler.Printf("passthrough %s %s", r.Method, path)

	var body io.Reader
	if r.Body != nil {
		body = r.Body
		defer r.Body.Close()
	}

	resp, err := h.server.forwardToGitHub(r.Context(), r.Method, path, body, r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusBadGateway)
		return
	}

	h.writeResponse(w, resp, respBody)
}

// forwardGraphQL forwards a GraphQL request without DIFC filtering.
func (h *proxyHandler) forwardGraphQL(w http.ResponseWriter, r *http.Request, _ string, body []byte) {
	resp, err := h.server.forwardToGitHub(r.Context(), http.MethodPost, "/graphql", bytes.NewReader(body), "application/json")
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusBadGateway)
		return
	}

	h.writeResponse(w, resp, respBody)
}

// writeResponse writes an upstream response to the client.
func (h *proxyHandler) writeResponse(w http.ResponseWriter, resp *http.Response, body []byte) {
	copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// writeEmptyResponse writes an empty JSON array or object response.
func (h *proxyHandler) writeEmptyResponse(w http.ResponseWriter, resp *http.Response) {
	copyResponseHeaders(w, resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write([]byte("[]"))
}

// copyResponseHeaders copies relevant headers from upstream to the client response.
func copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	// Copy rate limit headers
	for _, h := range []string{
		"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset",
		"X-RateLimit-Resource", "X-RateLimit-Used",
		"Link", // pagination
		"X-GitHub-Request-Id",
	} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
}
