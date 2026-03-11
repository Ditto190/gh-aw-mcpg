package guard

import (
	"context"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteSinkGuard_Name(t *testing.T) {
	g := NewWriteSinkGuard([]string{"private:github/gh-aw*"})
	assert.Equal(t, "write-sink", g.Name())
}

func TestWriteSinkGuard_LabelAgent(t *testing.T) {
	g := NewWriteSinkGuard([]string{"private:github/gh-aw*"})
	result, err := g.LabelAgent(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Agent.Secrecy, "write-sink should not set agent secrecy")
	assert.Empty(t, result.Agent.Integrity, "write-sink should not set agent integrity")
	assert.Equal(t, difc.ModeFilter, result.DIFCMode)
}

func TestWriteSinkGuard_LabelResource_UsesAcceptPatterns(t *testing.T) {
	accept := []string{"private:github/gh-aw*"}
	g := NewWriteSinkGuard(accept)

	resource, operation, err := g.LabelResource(context.Background(), "create_issue", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resource)

	// Operation must be Write
	assert.Equal(t, difc.OperationWrite, operation)

	// Secrecy must use the configured accept patterns
	secrecyTags := resource.Secrecy.Label.GetTags()
	assert.Len(t, secrecyTags, 1)
	assert.Contains(t, secrecyTags, difc.Tag("private:github/gh-aw*"))

	// Integrity must be empty (no requirements for writes)
	integrityTags := resource.Integrity.Label.GetTags()
	assert.Empty(t, integrityTags, "write-sink resource should have empty integrity")
}

func TestWriteSinkGuard_LabelResource_MultipleAcceptPatterns(t *testing.T) {
	accept := []string{"private:github/gh-aw*", "internal:github/copilot*"}
	g := NewWriteSinkGuard(accept)

	resource, operation, err := g.LabelResource(context.Background(), "noop", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resource)

	assert.Equal(t, difc.OperationWrite, operation)
	secrecyTags := resource.Secrecy.Label.GetTags()
	assert.Len(t, secrecyTags, 2)
	assert.Contains(t, secrecyTags, difc.Tag("private:github/gh-aw*"))
	assert.Contains(t, secrecyTags, difc.Tag("internal:github/copilot*"))
}

func TestWriteSinkGuard_LabelResource_EmptyAccept(t *testing.T) {
	g := NewWriteSinkGuard([]string{})

	resource, operation, err := g.LabelResource(context.Background(), "noop", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, resource)

	assert.Equal(t, difc.OperationWrite, operation)
	assert.Empty(t, resource.Secrecy.Label.GetTags(), "should be empty with no accept patterns")
	assert.Empty(t, resource.Integrity.Label.GetTags())
}

func TestWriteSinkGuard_LabelResponse(t *testing.T) {
	g := NewWriteSinkGuard([]string{"private:github/gh-aw*"})
	data, err := g.LabelResponse(context.Background(), "create_issue", nil, nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, data, "write-sink should not label responses")
}

func TestWriteSinkGuard_WriteEvaluation_Passes(t *testing.T) {
	// End-to-end: simulate the exact DIFC flow that was failing with noop guard.
	// Agent has secrecy from reading a private repo; write-sink accepts it.
	accept := []string{"private:github/gh-aw*"}
	g := NewWriteSinkGuard(accept)

	agentSecrecyTags := []difc.Tag{"private:github/gh-aw*"}
	agentIntegrityTags := []difc.Tag{
		"none:github/gh-aw*",
		"unapproved:github/gh-aw*",
		"approved:github/gh-aw*",
	}

	agentSecrecy := difc.NewSecrecyLabelWithTags(agentSecrecyTags)
	agentIntegrity := difc.NewIntegrityLabelWithTags(agentIntegrityTags)

	// Guard labels the resource using configured accept patterns
	resource, operation, err := g.LabelResource(context.Background(), "create_issue", nil, nil, nil)
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

func TestWriteSinkGuard_SecrecyMismatchFails(t *testing.T) {
	// If the agent has secrecy tags not covered by the accept patterns, write fails
	accept := []string{"private:github/gh-aw*"}
	g := NewWriteSinkGuard(accept)

	// Agent accessed a different private repo not in accept list
	agentSecrecyTags := []difc.Tag{"private:github/gh-aw*", "private:github/secret-repo"}
	agentSecrecy := difc.NewSecrecyLabelWithTags(agentSecrecyTags)
	agentIntegrity := difc.NewIntegrityLabelWithTags(nil)

	resource, operation, err := g.LabelResource(context.Background(), "create_issue", nil, nil, nil)
	require.NoError(t, err)

	evaluator := difc.NewEvaluatorWithMode(difc.EnforcementFilter)
	result := evaluator.Evaluate(agentSecrecy, agentIntegrity, resource, operation)

	assert.False(t, result.IsAllowed(), "write should fail: agent has secrecy tag not in accept list")
}
