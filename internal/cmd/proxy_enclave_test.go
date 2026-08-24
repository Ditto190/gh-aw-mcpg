package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validEnclaveProxyPolicy = `{
	"version": 1,
	"profile": "issues-read-v1",
	"audience": "gh-aw-enclave-github",
	"workflow_run_id": "run-123",
	"repositories": [{"repo": "assigned/private", "sensitivity": "confidential"}],
	"public_min_integrity": "approved",
	"allowed_operations": ["issues.get", "issues.list"],
	"max_capability_ttl_seconds": 600
}`

func TestResolveEnclaveProxyConfig(t *testing.T) {
	key := strings.Repeat("42", 32)
	config, guardPolicy, enabled, err := resolveEnclaveProxyConfig(
		validEnclaveProxyPolicy,
		key,
		"",
		nil,
		nil,
	)

	require.NoError(t, err)
	require.True(t, enabled)
	require.NotNil(t, config)
	assert.Equal(t, "issues-read-v1", config.Policy.Profile)
	assert.JSONEq(t, `{
		"allow-only": {
			"repos": ["assigned/private", "public"],
			"min-integrity": "approved"
		}
	}`, guardPolicy)
}

func TestResolveEnclaveProxyConfigRejectsIncompleteOrConflictingStartup(t *testing.T) {
	key := strings.Repeat("42", 32)
	secretKey := strings.Repeat("ab", 32)
	tests := []struct {
		name         string
		policy       string
		key          string
		explicit     string
		trustedBots  []string
		trustedUsers []string
		wantEnabled  bool
		wantErr      string
		notInError   string
	}{
		{name: "disabled", wantEnabled: false},
		{name: "missing key", policy: validEnclaveProxyPolicy, wantEnabled: true, wantErr: "must be configured together"},
		{name: "missing policy", key: key, wantEnabled: true, wantErr: "must be configured together"},
		{name: "explicit guard policy", policy: validEnclaveProxyPolicy, key: key, explicit: `{}`, wantEnabled: true, wantErr: "cannot be combined"},
		{name: "trusted bots", policy: validEnclaveProxyPolicy, key: key, trustedBots: []string{"bot"}, wantEnabled: true, wantErr: "not supported"},
		{name: "trusted users", policy: validEnclaveProxyPolicy, key: key, trustedUsers: []string{"user"}, wantEnabled: true, wantErr: "not supported"},
		{name: "invalid key", policy: validEnclaveProxyPolicy, key: secretKey + "0", wantEnabled: true, wantErr: "64 lowercase hex", notInError: secretKey},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, guardPolicy, enabled, err := resolveEnclaveProxyConfig(
				test.policy,
				test.key,
				test.explicit,
				test.trustedBots,
				test.trustedUsers,
			)
			assert.Equal(t, test.wantEnabled, enabled)
			if test.wantErr == "" {
				require.NoError(t, err)
				assert.Nil(t, config)
				assert.Empty(t, guardPolicy)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
			if test.notInError != "" {
				assert.NotContains(t, err.Error(), test.notInError)
			}
			assert.Nil(t, config)
			assert.Empty(t, guardPolicy)
		})
	}
}
