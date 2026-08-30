package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionIsolationTestGuard struct {
	instances *atomic.Int32
	closed    *atomic.Int32
	id        int32
}

type nonIsolatedTestGuard struct {
	guard.Guard
}

func (g *sessionIsolationTestGuard) Name() string {
	return "session-isolation-test"
}

func (g *sessionIsolationTestGuard) NewSessionGuard(context.Context) (guard.Guard, error) {
	return &sessionIsolationTestGuard{
		instances: g.instances,
		closed:    g.closed,
		id:        g.instances.Add(1),
	}, nil
}

func (g *sessionIsolationTestGuard) Close(context.Context) error {
	g.closed.Add(1)
	return nil
}

func (g *sessionIsolationTestGuard) LabelAgent(context.Context, interface{}, guard.BackendCaller, *difc.Capabilities) (*guard.LabelAgentResult, error) {
	return &guard.LabelAgentResult{DIFCMode: difc.ModeStrict}, nil
}

func (g *sessionIsolationTestGuard) LabelResource(context.Context, string, interface{}, guard.BackendCaller, *difc.Capabilities) (*difc.LabeledResource, difc.OperationType, error) {
	return difc.NewLabeledResource("test"), difc.OperationRead, nil
}

func (g *sessionIsolationTestGuard) LabelResponse(context.Context, string, interface{}, guard.BackendCaller, *difc.Capabilities) (difc.LabeledData, error) {
	return nil, nil
}

func TestGuardForSession_IsolatesMultiAgentInstances(t *testing.T) {
	var instances atomic.Int32
	var closed atomic.Int32
	template := &sessionIsolationTestGuard{instances: &instances, closed: &closed}
	us := &UnifiedServer{
		cfg: &config.Config{Gateway: &config.GatewayConfig{
			AgentIDs: []string{"primary", "enclave"},
		}},
		sessions:      make(map[string]*Session),
		guardRegistry: guard.NewRegistry(),
	}
	us.guardRegistry.Register("github", template)

	const callsPerAgent = 8
	type guardResult struct {
		instance *sessionIsolationTestGuard
		err      error
	}
	results := make(chan guardResult, callsPerAgent*2)
	var wg sync.WaitGroup
	for _, agentID := range []string{"primary", "enclave"} {
		for range callsPerAgent {
			wg.Add(1)
			go func() {
				defer wg.Done()
				instance, err := us.guardForSession(context.Background(), agentID, "github")
				if err != nil {
					results <- guardResult{err: err}
					return
				}
				typed, ok := instance.(*sessionIsolationTestGuard)
				if !ok {
					results <- guardResult{err: assert.AnError}
					return
				}
				results <- guardResult{instance: typed}
			}()
		}
	}
	wg.Wait()
	close(results)

	seen := make(map[int32]int)
	for result := range results {
		require.NoError(t, result.err)
		seen[result.instance.id]++
	}
	assert.Equal(t, int32(2), instances.Load())
	assert.Len(t, seen, 2)
	for _, count := range seen {
		assert.Equal(t, callsPerAgent, count)
	}

	primary, err := us.guardForSession(context.Background(), "primary", "github")
	require.NoError(t, err)
	enclave, err := us.guardForSession(context.Background(), "enclave", "github")
	require.NoError(t, err)
	assert.NotSame(t, primary, enclave)

	us.closeSessionGuards(context.Background())
	assert.Equal(t, int32(2), closed.Load())
}

func TestGuardForSession_SingularUsesRegisteredGuard(t *testing.T) {
	var instances atomic.Int32
	var closed atomic.Int32
	template := &sessionIsolationTestGuard{instances: &instances, closed: &closed}
	us := &UnifiedServer{
		cfg:           &config.Config{Gateway: &config.GatewayConfig{AgentID: "primary"}},
		guardRegistry: guard.NewRegistry(),
	}
	us.guardRegistry.Register("github", template)

	instance, err := us.guardForSession(context.Background(), "primary", "github")
	require.NoError(t, err)
	assert.Same(t, template, instance)
	assert.Zero(t, instances.Load())
}

func TestGuardForSession_MultiAgentRejectsNonIsolatedGuard(t *testing.T) {
	us := &UnifiedServer{
		cfg: &config.Config{Gateway: &config.GatewayConfig{
			AgentIDs: []string{"primary", "enclave"},
		}},
		sessions:      make(map[string]*Session),
		guardRegistry: guard.NewRegistry(),
	}
	us.guardRegistry.Register("github", &nonIsolatedTestGuard{Guard: guard.NewNoopGuard()})

	_, err := us.guardForSession(context.Background(), "primary", "github")
	require.Error(t, err)
	assert.ErrorContains(t, err, "does not support isolated session instances")
}
