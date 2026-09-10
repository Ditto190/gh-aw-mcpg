package server

import (
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseServerGuardPolicy_AllowOnly tests config.ParseServerGuardPolicy
// against the shapes of guard-policies map that the server package hands it,
// covering both the modern allow-only/write-sink format and the legacy
// repos/min-integrity format.
func TestParseServerGuardPolicy_AllowOnly(t *testing.T) {
	tests := []struct {
		name          string
		guardPolicies map[string]interface{}
		check         func(t *testing.T, policy *config.GuardPolicy)
	}{
		{
			// This is the exact format from smoke-allowonly.lock.yml.
			name: "modern allow-only format with slice repos",
			guardPolicies: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"min-integrity": "approved",
					"repos":         []interface{}{"github/gh-aw*"},
				},
			},
			check: func(t *testing.T, policy *config.GuardPolicy) {
				require.NotNil(t, policy.AllowOnly, "policy.AllowOnly should not be nil")
				assert.Equal(t, "approved", policy.AllowOnly.MinIntegrity)

				reposSlice, ok := policy.AllowOnly.Repos.([]interface{})
				require.True(t, ok, "repos should be []interface{}, got %T: %v", policy.AllowOnly.Repos, policy.AllowOnly.Repos)
				require.Len(t, reposSlice, 1)
				assert.Equal(t, "github/gh-aw*", reposSlice[0])
			},
		},
		{
			name: "modern allow-only format with string repos",
			guardPolicies: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"min-integrity": "merged",
					"repos":         "public",
				},
			},
			check: func(t *testing.T, policy *config.GuardPolicy) {
				require.NotNil(t, policy.AllowOnly)
				assert.Equal(t, "merged", policy.AllowOnly.MinIntegrity)
				assert.Equal(t, "public", policy.AllowOnly.Repos)
			},
		},
		{
			name: "legacy repos/min-integrity format at top level",
			guardPolicies: map[string]interface{}{
				"repos":         "all",
				"min-integrity": "none",
			},
			check: func(t *testing.T, policy *config.GuardPolicy) {
				require.NotNil(t, policy.AllowOnly)
				assert.Equal(t, "all", policy.AllowOnly.Repos)
				assert.Equal(t, "none", policy.AllowOnly.MinIntegrity)
			},
		},
		{
			name: "write-sink format",
			guardPolicies: map[string]interface{}{
				"write-sink": map[string]interface{}{
					"accept": []interface{}{"private:myorg"},
				},
			},
			check: func(t *testing.T, policy *config.GuardPolicy) {
				require.NotNil(t, policy.WriteSink, "policy.WriteSink should not be nil")
				assert.Nil(t, policy.AllowOnly, "write-sink policy should not set AllowOnly")
				assert.Equal(t, []string{"private:myorg"}, policy.WriteSink.Accept)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := config.ParseServerGuardPolicy("github", tt.guardPolicies)
			require.NoError(t, err, "config.ParseServerGuardPolicy should not return error")
			require.NotNil(t, policy, "policy should not be nil")
			tt.check(t, policy)
		})
	}
}

// TestParseServerGuardPolicy_Errors verifies that malformed guard-policies
// maps are rejected with a descriptive error instead of silently producing a
// nil or partially-populated policy.
func TestParseServerGuardPolicy_Errors(t *testing.T) {
	tests := []struct {
		name          string
		guardPolicies map[string]interface{}
		wantErr       string
	}{
		{
			name: "invalid repos scope value",
			guardPolicies: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"min-integrity": "approved",
					"repos":         "not-a-valid-scope",
				},
			},
			wantErr: "repos",
		},
		{
			name: "server-id-nested value is not an object",
			guardPolicies: map[string]interface{}{
				"github": "not-a-map",
			},
			wantErr: "expected object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := config.ParseServerGuardPolicy("github", tt.guardPolicies)
			require.Error(t, err)
			assert.Nil(t, policy)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestResolveGuardPolicy_ServerGuardPolicies tests resolving policy from server guard-policies
func TestResolveGuardPolicy_ServerGuardPolicies(t *testing.T) {
	cfg := &config.Config{
		DIFCMode: "strict",
		Servers: map[string]*config.ServerConfig{
			"github": {
				Type: "stdio",
				GuardPolicies: map[string]interface{}{
					"allow-only": map[string]interface{}{
						"min-integrity": "approved",
						"repos":         []interface{}{"github/gh-aw*"},
					},
				},
			},
		},
	}

	us := &UnifiedServer{
		cfg: cfg,
	}

	policy, source, err := us.resolveGuardPolicy("github")
	require.NoError(t, err, "resolveGuardPolicy should not return error")
	require.NotNil(t, policy, "policy should not be nil")
	assert.Equal(t, "server", source, "source should be 'server'")
	require.NotNil(t, policy.AllowOnly, "policy.AllowOnly should not be nil")
	assert.Equal(t, "approved", policy.AllowOnly.MinIntegrity, "MinIntegrity should be 'approved'")
}
