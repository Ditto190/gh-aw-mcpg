package guard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizePolicyPayload(t *testing.T) {
	tests := []struct {
		name    string
		policy  interface{}
		wantErr string
	}{
		{
			name:    "nil policy returns error",
			policy:  nil,
			wantErr: "policy is required",
		},
		{
			name:    "empty string returns error",
			policy:  "",
			wantErr: "policy string is empty",
		},
		{
			name:    "whitespace string returns error",
			policy:  "   ",
			wantErr: "policy string is empty",
		},
		{
			name:    "invalid JSON string returns error",
			policy:  "not-json",
			wantErr: "policy string is not valid JSON object",
		},
		{
			name:    "JSON array string rejected",
			policy:  `["a","b"]`,
			wantErr: "policy JSON must decode to an object",
		},
		{
			name:    "JSON string literal rejected",
			policy:  `"hello"`,
			wantErr: "policy JSON must decode to an object",
		},
		{
			name:    "JSON number literal rejected",
			policy:  `42`,
			wantErr: "policy JSON must decode to an object",
		},
		{
			name:   "valid JSON object string accepted",
			policy: `{"allow-only":{"repos":"all","min-integrity":"none"}}`,
		},
		{
			name:   "non-string policy passed through",
			policy: map[string]interface{}{"allow-only": "value"},
		},
		{
			name:   "non-string bool passed through",
			policy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePolicyPayload(tt.policy)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, got)
			}
		})
	}
}

func TestValidateStringArray(t *testing.T) {
	tests := []struct {
		name            string
		fieldName       string
		raw             interface{}
		requireNonEmpty bool
		wantErr         string
	}{
		{
			name:      "valid array of strings",
			fieldName: "blocked-users",
			raw:       []interface{}{"alice", "bob"},
			wantErr:   "",
		},
		{
			name:      "empty array allowed when requireNonEmpty=false",
			fieldName: "blocked-users",
			raw:       []interface{}{},
			wantErr:   "",
		},
		{
			name:            "empty array rejected when requireNonEmpty=true",
			fieldName:       "trusted-bots",
			raw:             []interface{}{},
			requireNonEmpty: true,
			wantErr:         "must be a non-empty array when present",
		},
		{
			name:      "non-array rejected",
			fieldName: "blocked-users",
			raw:       "not-an-array",
			wantErr:   "expected array of strings",
		},
		{
			name:            "non-array rejected with requireNonEmpty",
			fieldName:       "trusted-bots",
			raw:             42,
			requireNonEmpty: true,
			wantErr:         "expected non-empty array of strings",
		},
		{
			name:      "array with empty string rejected",
			fieldName: "blocked-users",
			raw:       []interface{}{"alice", ""},
			wantErr:   "each entry must be a non-empty string",
		},
		{
			name:      "array with whitespace-only string rejected",
			fieldName: "blocked-users",
			raw:       []interface{}{"  "},
			wantErr:   "each entry must be a non-empty string",
		},
		{
			name:      "array with non-string entry rejected",
			fieldName: "approval-labels",
			raw:       []interface{}{"valid", 42},
			wantErr:   "each entry must be a non-empty string",
		},
		{
			name:      "nil raw rejected",
			fieldName: "blocked-users",
			raw:       nil,
			wantErr:   "expected array of strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStringArray(tt.fieldName, tt.raw, tt.requireNonEmpty)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildStrictLabelAgentPayload(t *testing.T) {
	validPolicy := map[string]interface{}{
		"allow-only": map[string]interface{}{
			"repos":         "all",
			"min-integrity": "none",
		},
	}

	tests := []struct {
		name    string
		policy  interface{}
		wantErr string
	}{
		{
			name:    "nil policy returns error",
			policy:  nil,
			wantErr: "invalid guard policy transport shape",
		},
		{
			name: "legacy policy envelope rejected",
			policy: map[string]interface{}{
				"policy": map[string]interface{}{
					"allow-only": map[string]interface{}{
						"repos":         "all",
						"min-integrity": "none",
					},
				},
			},
			wantErr: "gateway policy adapter is outdated",
		},
		{
			name: "missing allow-only key returns error",
			policy: map[string]interface{}{
				"something-else": "value",
			},
			wantErr: "label_agent policy must use top-level allow-only object",
		},
		{
			name: "unexpected top-level key returns error",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
				},
				"extra-key": "value",
			},
			wantErr: `unexpected key "extra-key"`,
		},
		{
			name: "allow-only not an object returns error",
			policy: map[string]interface{}{
				"allow-only": "not-an-object",
			},
			wantErr: "invalid guard policy transport shape",
		},
		{
			name: "missing repos field returns error",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"min-integrity": "none",
				},
			},
			wantErr: "missing required fields repos and/or min-integrity",
		},
		{
			name: "missing min-integrity field returns error",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos": "all",
				},
			},
			wantErr: "missing required fields repos and/or min-integrity",
		},
		{
			name: "invalid repos value returns error",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "invalid",
					"min-integrity": "none",
				},
			},
			wantErr: "invalid repos value",
		},
		{
			name: "invalid min-integrity value returns error",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "invalid-level",
				},
			},
			wantErr: "invalid min-integrity value",
		},
		{
			name:   "valid policy with repos=all",
			policy: validPolicy,
		},
		{
			name: "valid policy with repos=public",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "public",
					"min-integrity": "approved",
				},
			},
		},
		{
			name: "valid policy with repos array",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         []interface{}{"owner/repo1", "owner/repo2"},
					"min-integrity": "merged",
				},
			},
		},
		{
			name: "valid policy using legacy integrity key",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":     "all",
					"integrity": "unapproved",
				},
			},
		},
		{
			name: "valid policy using legacy allowonly key",
			policy: map[string]interface{}{
				"allowonly": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
				},
			},
		},
		{
			name: "valid blocked-users array",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
					"blocked-users": []interface{}{"badbot", "spammer"},
				},
			},
		},
		{
			name: "invalid blocked-users array",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
					"blocked-users": []interface{}{42},
				},
			},
			wantErr: "invalid blocked-users value",
		},
		{
			name: "valid approval-labels array",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":           "all",
					"min-integrity":   "none",
					"approval-labels": []interface{}{"approved", "lgtm"},
				},
			},
		},
		{
			name: "invalid approval-labels array",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":           "all",
					"min-integrity":   "none",
					"approval-labels": "not-an-array",
				},
			},
			wantErr: "invalid approval-labels value",
		},
		{
			name: "valid trusted-bots at top level",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
				},
				"trusted-bots": []interface{}{"dependabot", "renovate"},
			},
		},
		{
			name: "empty trusted-bots rejected",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
				},
				"trusted-bots": []interface{}{},
			},
			wantErr: "invalid trusted-bots value",
		},
		{
			name: "valid trusted-users in allow-only",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
					"trusted-users": []interface{}{"alice", "bob"},
				},
			},
		},
		{
			name: "invalid trusted-users in allow-only",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
					"trusted-users": "not-an-array",
				},
			},
			wantErr: "invalid trusted-users value",
		},
		{
			name: "valid endorsement-reactions",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":                 "all",
					"min-integrity":         "none",
					"endorsement-reactions": []interface{}{"+1", "rocket"},
				},
			},
		},
		{
			name: "invalid disapproval-reactions",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":                  "all",
					"min-integrity":          "none",
					"disapproval-reactions":  []interface{}{},
					"endorsement-reactions":  []interface{}{"+1"},
				},
			},
		},
		{
			name: "valid disapproval-integrity",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":                "all",
					"min-integrity":        "none",
					"disapproval-integrity": "approved",
				},
			},
		},
		{
			name: "invalid disapproval-integrity",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":                "all",
					"min-integrity":        "none",
					"disapproval-integrity": "bad-value",
				},
			},
			wantErr: "invalid disapproval-integrity value",
		},
		{
			name: "valid endorser-min-integrity",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":                 "all",
					"min-integrity":         "none",
					"endorser-min-integrity": "merged",
				},
			},
		},
		{
			name: "invalid endorser-min-integrity",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":                 "all",
					"min-integrity":         "none",
					"endorser-min-integrity": 42,
				},
			},
			wantErr: "invalid endorser-min-integrity value",
		},
		{
			name: "unexpected allow-only key rejected",
			policy: map[string]interface{}{
				"allow-only": map[string]interface{}{
					"repos":         "all",
					"min-integrity": "none",
					"unknown-field": "value",
				},
			},
			wantErr: `unexpected allow-only key "unknown-field"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildStrictLabelAgentPayload(tt.policy)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, got)
			}
		})
	}
}

func TestBuildLabelAgentPayload(t *testing.T) {
	basePolicy := map[string]interface{}{
		"allow-only": map[string]interface{}{
			"repos":         "all",
			"min-integrity": "none",
		},
	}

	t.Run("no trusted bots or users returns policy as-is", func(t *testing.T) {
		result := BuildLabelAgentPayload(basePolicy, nil, nil)
		assert.Equal(t, basePolicy, result)
	})

	t.Run("empty slices returns policy as-is", func(t *testing.T) {
		result := BuildLabelAgentPayload(basePolicy, []string{}, []string{})
		assert.Equal(t, basePolicy, result)
	})

	t.Run("trusted bots injected at top level", func(t *testing.T) {
		result := BuildLabelAgentPayload(basePolicy, []string{"dependabot", "renovate"}, nil)
		payloadMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		bots, ok := payloadMap["trusted-bots"]
		require.True(t, ok)
		botsSlice, ok := bots.([]interface{})
		require.True(t, ok)
		assert.Len(t, botsSlice, 2)
		assert.Equal(t, "dependabot", botsSlice[0])
		assert.Equal(t, "renovate", botsSlice[1])
	})

	t.Run("trusted users injected into allow-only", func(t *testing.T) {
		result := BuildLabelAgentPayload(basePolicy, nil, []string{"alice", "bob"})
		payloadMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		allowOnly, ok := payloadMap["allow-only"].(map[string]interface{})
		require.True(t, ok)
		users, ok := allowOnly["trusted-users"]
		require.True(t, ok)
		usersSlice, ok := users.([]interface{})
		require.True(t, ok)
		assert.Len(t, usersSlice, 2)
		assert.Equal(t, "alice", usersSlice[0])
	})

	t.Run("both trusted bots and users injected", func(t *testing.T) {
		result := BuildLabelAgentPayload(basePolicy, []string{"bot1"}, []string{"user1"})
		payloadMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, payloadMap, "trusted-bots")
		allowOnly, ok := payloadMap["allow-only"].(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, allowOnly, "trusted-users")
	})

	t.Run("trusted users not injected when allow-only absent", func(t *testing.T) {
		policyWithoutAllowOnly := map[string]interface{}{
			"something": "value",
		}
		result := BuildLabelAgentPayload(policyWithoutAllowOnly, nil, []string{"user1"})
		payloadMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		// trusted-users should NOT be at top level (only in allow-only)
		assert.NotContains(t, payloadMap, "trusted-users")
	})

	t.Run("invalid policy falls back to original", func(t *testing.T) {
		// A channel cannot be JSON-marshaled, so PolicyToMap will fail
		type unserializable struct{ C chan int }
		// We use a string policy here that would fail PolicyToMap
		result := BuildLabelAgentPayload("not-a-json-object", []string{"bot"}, nil)
		// Should return original policy when conversion fails
		assert.Equal(t, "not-a-json-object", result)
	})
}

func TestIsValidAllowOnlyRepos(t *testing.T) {
	tests := []struct {
		name  string
		repos interface{}
		want  bool
	}{
		{name: "string all", repos: "all", want: true},
		{name: "string ALL uppercase", repos: "ALL", want: true},
		{name: "string public", repos: "public", want: true},
		{name: "string PUBLIC uppercase", repos: "PUBLIC", want: true},
		{name: "string all with spaces", repos: "  all  ", want: true},
		{name: "string invalid", repos: "private", want: false},
		{name: "string empty", repos: "", want: false},
		{name: "array with strings", repos: []interface{}{"owner/repo"}, want: true},
		{name: "array with multiple strings", repos: []interface{}{"owner/repo1", "owner/repo2"}, want: true},
		{name: "empty array rejected", repos: []interface{}{}, want: false},
		{name: "array with non-string", repos: []interface{}{42}, want: false},
		{name: "array with mixed types", repos: []interface{}{"owner/repo", 42}, want: false},
		{name: "nil", repos: nil, want: false},
		{name: "integer", repos: 42, want: false},
		{name: "bool", repos: true, want: false},
		{name: "map", repos: map[string]interface{}{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidAllowOnlyRepos(tt.repos)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckBoolFailure(t *testing.T) {
	tests := []struct {
		name       string
		raw        map[string]interface{}
		resultJSON []byte
		key        string
		wantErr    string
	}{
		{
			name:       "key absent - no failure",
			raw:        map[string]interface{}{},
			resultJSON: []byte(`{}`),
			key:        "success",
			wantErr:    "",
		},
		{
			name:       "key true - no failure",
			raw:        map[string]interface{}{"success": true},
			resultJSON: []byte(`{"success":true}`),
			key:        "success",
			wantErr:    "",
		},
		{
			name:       "key false with error message",
			raw:        map[string]interface{}{"success": false, "error": "policy rejected"},
			resultJSON: []byte(`{"success":false,"error":"policy rejected"}`),
			key:        "success",
			wantErr:    "label_agent rejected policy: policy rejected",
		},
		{
			name:       "key false without error message",
			raw:        map[string]interface{}{"success": false},
			resultJSON: []byte(`{"success":false}`),
			key:        "success",
			wantErr:    "label_agent returned non-success status",
		},
		{
			name:       "key false with empty error message",
			raw:        map[string]interface{}{"success": false, "error": ""},
			resultJSON: []byte(`{"success":false,"error":""}`),
			key:        "success",
			wantErr:    "label_agent returned non-success status",
		},
		{
			name:       "key false with whitespace error message",
			raw:        map[string]interface{}{"success": false, "error": "   "},
			resultJSON: []byte(`{"success":false,"error":"   "}`),
			key:        "success",
			wantErr:    "label_agent returned non-success status",
		},
		{
			name:       "key is non-bool value - treated as absent",
			raw:        map[string]interface{}{"success": "true"},
			resultJSON: []byte(`{"success":"true"}`),
			key:        "success",
			wantErr:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkBoolFailure(tt.raw, tt.resultJSON, tt.key)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
