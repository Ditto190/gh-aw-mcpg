package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/delegation"
	"github.com/github/gh-aw-mcpg/internal/sanitize"
	"github.com/github/gh-aw-mcpg/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartUnifiedDelegationControlServesStatus(t *testing.T) {
	previousRedaction := sanitize.PrivateSelectorRedactionEnabled()
	t.Cleanup(func() { sanitize.SetPrivateSelectorRedaction(previousRedaction) })

	const capabilityKey = "control-capability-key-32-bytes!!"
	delegationConfig, err := testRuntimeDelegationConfig(t, availableLoopbackAddr(t), capabilityKey)
	require.NoError(t, err)
	cfg := &config.Config{
		Servers:    map[string]*config.ServerConfig{},
		Delegation: delegationConfig,
	}
	us, err := server.NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	defer us.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controlServer, controlErrCh, err := startUnifiedDelegationControl(ctx, cancel, us, delegationConfig)
	require.NoError(t, err)
	require.NotNil(t, controlServer)
	defer func() { _ = controlServer.Shutdown(context.Background()) }()

	body := bytes.NewBufferString(`{"run_id":"run-1","enclave_entry_id":"entry-1"}`)
	req, err := http.NewRequest(http.MethodPost, "http://"+delegationConfig.ControlListenAddr+delegation.ControlPathPrefix+"status", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", capabilityKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.InEpsilon(t, 1.0, payload["generation"], 0)
	assert.NoError(t, selectDelegationControlError(nil, controlErrCh))
}

func TestStartUnifiedDelegationControlFailsWhenListenAddrOccupied(t *testing.T) {
	previousRedaction := sanitize.PrivateSelectorRedactionEnabled()
	t.Cleanup(func() { sanitize.SetPrivateSelectorRedaction(previousRedaction) })

	const capabilityKey = "control-capability-key-32-bytes!!"
	occupiedAddr := availableLoopbackAddr(t)
	occupyingListener, err := net.Listen("tcp", occupiedAddr)
	require.NoError(t, err)
	defer occupyingListener.Close()

	delegationConfig, err := testRuntimeDelegationConfig(t, occupiedAddr, capabilityKey)
	require.NoError(t, err)
	cfg := &config.Config{
		Servers:    map[string]*config.ServerConfig{},
		Delegation: delegationConfig,
	}
	us, err := server.NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	defer us.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controlServer, _, err := startUnifiedDelegationControl(ctx, cancel, us, delegationConfig)
	require.Error(t, err)
	assert.Nil(t, controlServer)
	assert.Contains(t, err.Error(), "failed to listen on private delegation control channel")
}

func testRuntimeDelegationConfig(t *testing.T, listenAddr, capabilityKey string) (*delegation.RuntimeConfig, error) {
	t.Helper()
	envelope := &delegation.Envelope{
		RunID:               "run-1",
		EnclaveBackend:      "awf-enclave",
		AllowedRepositories: []string{"github/gh-aw"},
		ToolPolicy:          delegation.ToolPolicyGitHubRepositoryReadV1,
		AllowedSchemaHashes: []string{"sha256:test"},
		MaxIdentityTTL:      120 * time.Second,
		ExpiresAt:           time.Now().Add(time.Hour),
	}
	store, err := delegation.NewStore(envelope, 1)
	if err != nil {
		return nil, err
	}
	capability, err := delegation.NewControlCapability(capabilityKey)
	if err != nil {
		return nil, err
	}
	return &delegation.RuntimeConfig{
		Store:             store,
		Capability:        capability,
		StatePath:         t.TempDir() + "/state.json",
		ControlListenAddr: listenAddr,
	}, nil
}
