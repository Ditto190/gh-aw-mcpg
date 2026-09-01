package guard

import (
	"context"
	"fmt"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logGuard = logger.ForFile()

// BackendCaller provides a way for guards to make read-only calls to the backend
// to gather information needed for labeling (e.g., fetching issue author)
type BackendCaller interface {
	// CallTool makes a read-only call to the backend MCP server
	// This is used by guards to gather metadata for labeling
	CallTool(ctx context.Context, toolName string, args interface{}) (interface{}, error)
}

// Guard handles DIFC labeling for a specific MCP server
// Guards ONLY label resources - they do NOT make access control decisions
// The Reference Monitor (in the server) uses guard-provided labels to enforce DIFC policies
type Guard interface {
	// Name returns the identifier for this guard (e.g., "github", "noop")
	Name() string

	// LabelAgent initializes guard policy and returns effective agent/session state
	// for the current session.
	// Returns:
	//   - result: effective labels, mode, and normalized policy
	//   - error: any validation/initialization error
	LabelAgent(ctx context.Context, policy interface{}, backend BackendCaller, caps *difc.Capabilities) (*LabelAgentResult, error)

	// LabelResource determines the resource being accessed and its labels
	// This may call the backend (via BackendCaller) to gather metadata needed for labeling
	// Returns:
	//   - resource: The labeled resource (simple or nested structure for fine-grained filtering)
	//   - operation: The type of operation (Read, Write, or ReadWrite)
	//   - error: Any error that occurred during labeling
	LabelResource(ctx context.Context, toolName string, args interface{}, backend BackendCaller, caps *difc.Capabilities) (*difc.LabeledResource, difc.OperationType, error)

	// LabelResponse labels the response data after a successful backend call
	// This is used for fine-grained filtering of collections
	// Returns:
	//   - labeledData: The response data with per-item labels (if applicable)
	//   - error: Any error that occurred during labeling
	// If the guard returns nil for labeledData, the reference monitor will use the
	// resource labels from LabelResource for the entire response
	LabelResponse(ctx context.Context, toolName string, result interface{}, backend BackendCaller, caps *difc.Capabilities) (difc.LabeledData, error)
}

// SessionGuardFactory creates an isolated guard instance for one authenticated
// session. Stateful guards must not share policy or failure state across agents.
type SessionGuardFactory interface {
	NewSessionGuard(ctx context.Context) (Guard, error)
}

// NewSessionGuard creates an isolated instance from a registered guard template.
func NewSessionGuard(ctx context.Context, template Guard) (Guard, error) {
	logGuard.Printf("NewSessionGuard: template=%s", template.Name())

	factory, ok := template.(SessionGuardFactory)
	if !ok {
		logGuard.Printf("NewSessionGuard: guard %q does not implement SessionGuardFactory, no per-session isolation available", template.Name())
		return nil, fmt.Errorf("guard %q does not support isolated session instances", template.Name())
	}

	sessionGuard, err := factory.NewSessionGuard(ctx)
	if err != nil {
		logGuard.Printf("NewSessionGuard: template=%s failed: %v", template.Name(), err)
		return nil, err
	}

	logGuard.Printf("NewSessionGuard: template=%s created isolated session guard", template.Name())
	return sessionGuard, nil
}

// LabelAgentResult describes the effective policy/session state returned by a guard.
type LabelAgentResult struct {
	Agent            AgentLabelsPayload     `json:"agent"`
	DIFCMode         string                 `json:"difc_mode"`
	NormalizedPolicy map[string]interface{} `json:"normalized_policy,omitempty"`
}

// AgentLabelsPayload holds effective secrecy/integrity labels for the session.
type AgentLabelsPayload struct {
	Secrecy   []string `json:"secrecy"`
	Integrity []string `json:"integrity"`
}

// RequestState represents any state that the guard needs to pass from request to response
// This is useful when the guard needs to carry information from LabelResource to LabelResponse
type RequestState interface{}

// IsSafeOutputsServer reports whether serverID identifies a safe-outputs server.
// Safe-outputs servers use write-sink guards and are subject to special DIFC
// sink-visibility policy (see internal/server/guard_init.go).
// Matches the canonical "safe-outputs" ID and the legacy "safeoutputs" alias.
func IsSafeOutputsServer(serverID string) bool {
	result := serverID == "safe-outputs" || serverID == "safeoutputs"
	logGuard.Printf("IsSafeOutputsServer: serverID=%q, result=%v", serverID, result)
	return result
}
