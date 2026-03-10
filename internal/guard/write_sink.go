package guard

import (
	"context"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/mcp"
)

var logWriteSink = logger.New("guard:write-sink")

// WriteSinkGuard is a guard for write-only output channels (e.g., safe-outputs).
//
// When DIFC is enabled, an agent that reads from a guarded server (like GitHub)
// acquires secrecy and integrity tags. Writing to an unguarded output server
// would fail because the DIFC evaluator sees a label mismatch: the agent has
// tags that the resource (with empty labels from a noop guard) does not.
//
// The write-sink guard fixes this by:
//   - Returning empty labels from LabelAgent (does not contribute agent labels)
//   - Mirroring the agent's current secrecy/integrity tags onto the resource
//   - Classifying all operations as OperationWrite
//
// This ensures writes always succeed because:
//   - Integrity: resource requires no tags (empty), agent has all zero required → OK
//   - Secrecy: resource secrecy = agent secrecy, so agentSecrecy ⊆ resourceSecrecy → OK
type WriteSinkGuard struct{}

// NewWriteSinkGuard creates a new write-sink guard
func NewWriteSinkGuard() *WriteSinkGuard {
	logWriteSink.Print("Creating new write-sink guard")
	return &WriteSinkGuard{}
}

// Name returns the identifier for this guard
func (g *WriteSinkGuard) Name() string {
	return "write-sink"
}

// LabelAgent returns empty labels. The write-sink does not contribute agent
// labels — those are set by the primary guard (e.g., the GitHub WASM guard).
func (g *WriteSinkGuard) LabelAgent(_ context.Context, _ interface{}, _ BackendCaller, _ *difc.Capabilities) (*LabelAgentResult, error) {
	logWriteSink.Print("LabelAgent: returning empty labels (write-sink does not label agents)")
	return &LabelAgentResult{
		Agent: AgentLabelsPayload{
			Secrecy:   []string{},
			Integrity: []string{},
		},
		DIFCMode: difc.ModeFilter,
	}, nil
}

// LabelResource mirrors the agent's current secrecy tags onto the resource
// and classifies the operation as a write.
//
// For writes the DIFC evaluator checks:
//   - agentSecrecy ⊆ resource.Secrecy     (no secret information leak)
//   - resource.Integrity ⊆ agentIntegrity  (agent is trusted enough)
//
// By copying the agent's secrecy onto the resource, the first check trivially
// passes. By leaving the resource integrity empty, the second check also passes
// because the agent has all zero of the (empty) required integrity tags.
func (g *WriteSinkGuard) LabelResource(ctx context.Context, toolName string, _ interface{}, _ BackendCaller, _ *difc.Capabilities) (*difc.LabeledResource, difc.OperationType, error) {
	logWriteSink.Printf("LabelResource: tool=%s, operation=write", toolName)

	// Read the agent's current tags from the request context.
	// These were placed there by callBackendTool before calling LabelResource.
	var secrecyTags []difc.Tag
	if snapshot, ok := mcp.GetAgentTagsSnapshotFromContext(ctx); ok {
		for _, s := range snapshot.Secrecy {
			secrecyTags = append(secrecyTags, difc.Tag(s))
		}
		logWriteSink.Printf("LabelResource: mirroring %d agent secrecy tags onto resource", len(secrecyTags))
	} else {
		logWriteSink.Print("LabelResource: no agent tags snapshot in context, using empty secrecy")
	}

	resource := &difc.LabeledResource{
		Description: "write-sink (" + toolName + ")",
		Secrecy:     *difc.NewSecrecyLabelWithTags(secrecyTags),
		Integrity:   *difc.NewIntegrityLabel(), // empty: no integrity requirements
	}

	return resource, difc.OperationWrite, nil
}

// LabelResponse returns nil; the write-sink does not perform fine-grained
// response labeling since all operations are writes (responses are confirmations).
func (g *WriteSinkGuard) LabelResponse(_ context.Context, _ string, _ interface{}, _ BackendCaller, _ *difc.Capabilities) (difc.LabeledData, error) {
	return nil, nil
}
