package guard

import (
	"context"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSinkGuard_Name(t *testing.T) {
	g := NewWriteSinkGuard()
	assert.Equal(t, "write-sink", g.Name())
}

func TestWriteSinkGuard_LabelAgent(t *testing.T) {
	g := NewWriteSinkGuard()
	result, err := g.LabelAgent(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Agent.Secrecy, "write-sink should not set agent secrecy")
	assert.Empty(t, result.Agent.Integrity, "write-sink should not set agent integrity")
	assert.Equal(t, difc.ModeFilter, result.DIFCMode)
}

func TestWriteSinkGuard_LabelResource_MirrorsAgentSecrecy(t *testing.T) {
	g := NewWriteSinkGuard()

	// Simulate agent with secrecy and integrity tags (set by GitHub guard)
	ctx := context.WithValue(context.Background(), mcp.AgentTagsSnapshotContextKey, &mcp.AgentTagsSnapshot{
		Secrecy:   []string{"private:github/gh-aw*"},
		Integrity: []string{"none:github/gh-aw*", "unapproved:github/gh-aw*", "approved:github/gh-aw*"},
	})

	resource, operation, err := g.LabelResource(ctx, "create_issue", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resource)

	// Operation must be Write
	assert.Equal(t, difc.OperationWrite, operation)

	// Secrecy must mirror agent's secrecy tags
	secrecyTags := resource.Secrecy.Label.GetTags()
	assert.Len(t, secrecyTags, 1)
	assert.Contains(t, secrecyTags, difc.Tag("private:github/gh-aw*"))

	// Integrity must be empty (no requirements for writes)
	integrityTags := resource.Integrity.Label.GetTags()
	assert.Empty(t, integrityTags, "write-sink resource should have empty integrity")
}

func TestWriteSinkGuard_LabelResource_NoAgentContext(t *testing.T) {
	g := NewWriteSinkGuard()

	// No agent tags in context (e.g., DIFC not active)
	resource, operation, err := g.LabelResource(context.Background(), "noop", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resource)

	assert.Equal(t, difc.OperationWrite, operation)
	assert.Empty(t, resource.Secrecy.Label.GetTags(), "should be empty when no agent context")
	assert.Empty(t, resource.Integrity.Label.GetTags())
}

func TestWriteSinkGuard_LabelResponse(t *testing.T) {
	g := NewWriteSinkGuard()
	data, err := g.LabelResponse(context.Background(), "create_issue", nil, nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, data, "write-sink should not label responses")
}

func TestWriteSinkGuard_WriteEvaluation_Passes(t *testing.T) {
	// End-to-end: simulate the exact DIFC flow that was failing with noop guard
	g := NewWriteSinkGuard()

	agentSecrecyTags := []difc.Tag{"private:github/gh-aw*"}
	agentIntegrityTags := []difc.Tag{
		"none:github/gh-aw*",
		"unapproved:github/gh-aw*",
		"approved:github/gh-aw*",
	}

	agentSecrecy := difc.NewSecrecyLabelWithTags(agentSecrecyTags)
	agentIntegrity := difc.NewIntegrityLabelWithTags(agentIntegrityTags)

	// Set up context with agent tags
	ctx := context.WithValue(context.Background(), mcp.AgentTagsSnapshotContextKey, &mcp.AgentTagsSnapshot{
		Secrecy:   []string{"private:github/gh-aw*"},
		Integrity: []string{"none:github/gh-aw*", "unapproved:github/gh-aw*", "approved:github/gh-aw*"},
	})

	// Guard labels the resource
	resource, operation, err := g.LabelResource(ctx, "create_issue", nil, nil, nil)
	require.NoError(t, err)

	// Evaluate with filter mode (same as production)
	evaluator := difc.NewEvaluatorWithMode(difc.EnforcementFilter)
	result := evaluator.Evaluate(agentSecrecy, agentIntegrity, resource, operation)

	assert.True(t, result.IsAllowed(), "write to sink should be allowed; got: %s", result.Reason)
}

func TestWriteSinkGuard_NoopWouldFail(t *testing.T) {
	// Demonstrate that noop guard would fail in the same scenario
	g := NewNoopGuard()

	agentSecrecyTags := []difc.Tag{"private:github/gh-aw*"}
	agentIntegrityTags := []difc.Tag{
		"none:github/gh-aw*",
		"unapproved:github/gh-aw*",
		"approved:github/gh-aw*",
	}

	agentSecrecy := difc.NewSecrecyLabelWithTags(agentSecrecyTags)
	agentIntegrity := difc.NewIntegrityLabelWithTags(agentIntegrityTags)

	resource, operation, err := g.LabelResource(context.Background(), "create_issue", nil, nil, nil)
	require.NoError(t, err)

	evaluator := difc.NewEvaluatorWithMode(difc.EnforcementFilter)
	result := evaluator.Evaluate(agentSecrecy, agentIntegrity, resource, operation)

	assert.False(t, result.IsAllowed(), "noop guard should cause DIFC violation with tainted agent")
	assert.Contains(t, result.Reason, "integrity")
}

func TestRegistryUpgradeNoopToWriteSink(t *testing.T) {
	r := NewRegistry()
	r.Register("github", &mockGuard{id: "github-wasm"})
	r.Register("safeoutputs", NewNoopGuard())
	r.Register("agenticworkflows", NewNoopGuard())

	// Before upgrade
	assert.Equal(t, "mock-github-wasm", r.Get("github").Name())
	assert.Equal(t, "noop", r.Get("safeoutputs").Name())
	assert.Equal(t, "noop", r.Get("agenticworkflows").Name())

	r.UpgradeNoopToWriteSink()

	// After upgrade: noop → write-sink, WASM unchanged
	assert.Equal(t, "mock-github-wasm", r.Get("github").Name())
	assert.Equal(t, "write-sink", r.Get("safeoutputs").Name())
	assert.Equal(t, "write-sink", r.Get("agenticworkflows").Name())
}
