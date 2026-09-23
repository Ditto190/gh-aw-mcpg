package delegation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateOrConfirmRequestWire_RequestedTTLIsSeconds(t *testing.T) {
	var wire CreateOrConfirmRequestWire
	require.NoError(t, json.Unmarshal([]byte(`{"requested_ttl":120}`), &wire))

	request, err := wire.ToRequest()
	require.NoError(t, err)
	assert.Equal(t, 120*time.Second, request.RequestedTTL)
}

func TestIdentityBindingRoundTrip(t *testing.T) {
	req := validRequest()
	req.InvocationExpiresAt = time.Now().Add(time.Minute).In(time.FixedZone("test", 3600))
	identity := &Identity{
		delegationBinding: bindingFromRequest(req),
		ExpiresAt:         time.Now().Add(30 * time.Second),
		IdempotencyKey:    req.IdempotencyKey,
		CreatedAt:         time.Now(),
	}

	restored := identity.toRequest()
	require.Equal(t, req.IdempotencyKey, restored.IdempotencyKey)
	assert.Equal(t, bindingFromRequest(req), bindingFromRequest(restored))
	assert.Equal(t, identity.ExpiresAt.Sub(identity.CreatedAt), restored.RequestedTTL)
}

func TestRequestCoreFieldsStayFlatOnTheWire(t *testing.T) {
	const wireJSON = `{"run_id":"run-1","enclave_backend":"awf-enclave","enclave_entry_id":"entry-1","invocation_id":"inv-1","repository":"github/gh-aw","tool_policy":"` + ToolPolicyGitHubRepositoryReadV1 + `","schema_hash":"sha256:abc","admitted_default_branch_sha":"deadbeef","requested_ttl":60,"idempotency_key":"idem-1"}`

	var wire CreateOrConfirmRequestWire
	require.NoError(t, json.Unmarshal([]byte(wireJSON), &wire))

	request, err := wire.ToRequest()
	require.NoError(t, err)

	assert := assert.New(t)
	assert.Equal("run-1", request.RunID)
	assert.Equal("awf-enclave", request.EnclaveBackend)
	assert.Equal("entry-1", request.EnclaveEntryID)
	assert.Equal("inv-1", request.InvocationID)
	assert.Equal("github/gh-aw", request.Repository)
	assert.Equal(ToolPolicyGitHubRepositoryReadV1, request.ToolPolicy)
	assert.Equal("sha256:abc", request.SchemaHash)
	assert.Equal("deadbeef", request.AdmittedDefaultBranchSHA)
	assert.Equal(60*time.Second, request.RequestedTTL)
	assert.Equal("idem-1", request.IdempotencyKey)

	identity := &Identity{delegationBinding: bindingFromRequest(request)}
	encoded, err := json.Marshal(identity)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal("run-1", decoded["run_id"])
	assert.Equal("github/gh-aw", decoded["repository"])
	assert.Equal("sha256:abc", decoded["schema_hash"])
	assert.NotContains(decoded, "RequestCore")
}
