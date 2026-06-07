package difc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildReflectResponse_EmptyRegistry(t *testing.T) {
	resp := BuildReflectResponse(DIFCComponents{
		Mode:          EnforcementFilter,
		AgentRegistry: NewAgentRegistry(),
	})

	require.NotNil(t, resp.Agents)
	assert.Empty(t, resp.Agents)
	assert.Equal(t, "filter", resp.Mode)
	_, err := time.Parse(time.RFC3339, resp.Timestamp)
	assert.NoError(t, err)
}

func TestBuildReflectResponse_SkipsNilAgentEntries(t *testing.T) {
	reg := NewAgentRegistry()
	reg.agents["broken"] = nil

	resp := BuildReflectResponse(DIFCComponents{
		Mode:          EnforcementStrict,
		AgentRegistry: reg,
	})

	assert.NotContains(t, resp.Agents, "broken")
}
