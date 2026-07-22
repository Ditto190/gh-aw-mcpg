package guard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// basePolicy returns a minimal valid allow-only policy map for testing.
func basePolicy() map[string]interface{} {
	return map[string]interface{}{
		"allow-only": map[string]interface{}{
			"repos":         "public",
			"min-integrity": "none",
		},
	}
}

// TestBuildLabelAgentPayload_NoBotsNoUsers tests that when both trustedBots and
// trustedUsers are empty, the policy is returned unchanged (no copy, same pointer).
func TestBuildLabelAgentPayload_NoBotsNoUsers(t *testing.T) {
	policy := basePolicy()
	result := BuildLabelAgentPayload(policy, nil, nil)
	assert.Equal(t, policy, result, "should return policy unchanged when no bots or users provided")
}

// TestBuildLabelAgentPayload_EmptySlicesReturnPolicyUnchanged tests that empty
// (non-nil) slices for both parameters also return the policy unchanged.
func TestBuildLabelAgentPayload_EmptySlicesReturnPolicyUnchanged(t *testing.T) {
	policy := basePolicy()
	result := BuildLabelAgentPayload(policy, []string{}, []string{})
	assert.Equal(t, policy, result, "should return policy unchanged when empty slices provided")
}

// TestBuildLabelAgentPayload_OnlyTrustedBots tests that trusted bots are injected
// as a top-level key in the payload when provided.
func TestBuildLabelAgentPayload_OnlyTrustedBots(t *testing.T) {
	policy := basePolicy()
	trustedBots := []string{"dependabot[bot]", "renovate[bot]"}

	result := BuildLabelAgentPayload(policy, trustedBots, nil)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "result should be a map")

	botsRaw, hasBots := resultMap["trusted-bots"]
	require.True(t, hasBots, "result should have trusted-bots key")

	bots, ok := botsRaw.([]interface{})
	require.True(t, ok, "trusted-bots should be []interface{}")
	require.Len(t, bots, 2)
	assert.Equal(t, "dependabot[bot]", bots[0])
	assert.Equal(t, "renovate[bot]", bots[1])

	// allow-only should still be present
	_, hasAllowOnly := resultMap["allow-only"]
	assert.True(t, hasAllowOnly, "allow-only should still be in result")

	// trusted-users should NOT be present in allow-only
	allowOnly, _ := resultMap["allow-only"].(map[string]interface{})
	_, hasTrustedUsers := allowOnly["trusted-users"]
	assert.False(t, hasTrustedUsers, "trusted-users should not be injected when not provided")
}

// TestBuildLabelAgentPayload_OnlyTrustedUsers tests that trusted users are injected
// inside the allow-only object when provided.
func TestBuildLabelAgentPayload_OnlyTrustedUsers(t *testing.T) {
	policy := basePolicy()
	trustedUsers := []string{"alice", "bob"}

	result := BuildLabelAgentPayload(policy, nil, trustedUsers)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "result should be a map")

	// trusted-bots should NOT be present at top-level
	_, hasBots := resultMap["trusted-bots"]
	assert.False(t, hasBots, "trusted-bots should not be present when not provided")

	allowOnly, ok := resultMap["allow-only"].(map[string]interface{})
	require.True(t, ok, "allow-only should be present and a map")

	usersRaw, hasUsers := allowOnly["trusted-users"]
	require.True(t, hasUsers, "trusted-users should be injected into allow-only")

	users, ok := usersRaw.([]interface{})
	require.True(t, ok, "trusted-users should be []interface{}")
	require.Len(t, users, 2)
	assert.Equal(t, "alice", users[0])
	assert.Equal(t, "bob", users[1])
}

// TestBuildLabelAgentPayload_BothBotsAndUsers tests that both trusted bots and
// trusted users are injected when both are provided.
func TestBuildLabelAgentPayload_BothBotsAndUsers(t *testing.T) {
	policy := basePolicy()
	trustedBots := []string{"dependabot[bot]"}
	trustedUsers := []string{"alice"}

	result := BuildLabelAgentPayload(policy, trustedBots, trustedUsers)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "result should be a map")

	// Verify trusted-bots at top-level
	botsRaw, hasBots := resultMap["trusted-bots"]
	require.True(t, hasBots, "trusted-bots should be present")
	bots, _ := botsRaw.([]interface{})
	require.Len(t, bots, 1)
	assert.Equal(t, "dependabot[bot]", bots[0])

	// Verify trusted-users inside allow-only
	allowOnly, ok := resultMap["allow-only"].(map[string]interface{})
	require.True(t, ok, "allow-only should be present and a map")

	usersRaw, hasUsers := allowOnly["trusted-users"]
	require.True(t, hasUsers, "trusted-users should be in allow-only")
	users, _ := usersRaw.([]interface{})
	require.Len(t, users, 1)
	assert.Equal(t, "alice", users[0])
}

// TestBuildLabelAgentPayload_StringPolicy tests that a JSON string policy is
// handled via GuardPolicyToMap (JSON roundtrip).
func TestBuildLabelAgentPayload_StringPolicyWithBots(t *testing.T) {
	policy := `{"allow-only":{"repos":"public","min-integrity":"none"}}`
	trustedBots := []string{"dependabot[bot]"}

	result := BuildLabelAgentPayload(policy, trustedBots, nil)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "result should be a map after JSON roundtrip")

	botsRaw, hasBots := resultMap["trusted-bots"]
	require.True(t, hasBots, "trusted-bots should be present")
	bots, _ := botsRaw.([]interface{})
	require.Len(t, bots, 1)
	assert.Equal(t, "dependabot[bot]", bots[0])
}

// TestBuildLabelAgentPayload_TrustedUsersNoAllowOnly tests that when the policy
// has no allow-only key, trusted-users injection is silently skipped but
// trusted-bots is still injected at top-level if provided.
func TestBuildLabelAgentPayload_TrustedUsersNoAllowOnly(t *testing.T) {
	// Policy with write-sink instead of allow-only — no allow-only key
	policy := map[string]interface{}{
		"write-sink": map[string]interface{}{
			"accept": []interface{}{"*"},
		},
	}
	trustedBots := []string{"dependabot[bot]"}
	trustedUsers := []string{"alice"}

	result := BuildLabelAgentPayload(policy, trustedBots, trustedUsers)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "result should be a map")

	// trusted-bots should be injected
	_, hasBots := resultMap["trusted-bots"]
	assert.True(t, hasBots, "trusted-bots should be injected at top-level")

	// write-sink should be present; no allow-only
	_, hasWriteSink := resultMap["write-sink"]
	assert.True(t, hasWriteSink, "write-sink should still be present")

	_, hasAllowOnly := resultMap["allow-only"]
	assert.False(t, hasAllowOnly, "allow-only should not exist in write-sink policy")
}

// TestBuildLabelAgentPayload_PolicyNotMutated tests that the original policy map
// is not mutated when bots and users are injected.
func TestBuildLabelAgentPayload_PolicyNotMutated(t *testing.T) {
	original := basePolicy()
	allowOnlyBefore := map[string]interface{}{
		"repos":         "public",
		"min-integrity": "none",
	}

	_ = BuildLabelAgentPayload(original, []string{"bot"}, []string{"user"})

	// Original allow-only should not have trusted-users
	allowOnly, _ := original["allow-only"].(map[string]interface{})
	_, hasTrustedUsers := allowOnly["trusted-users"]

	// Note: GuardPolicyToMap does a JSON roundtrip, so a new map is created.
	// The original map's allow-only won't be mutated.
	assert.False(t, hasTrustedUsers, "original policy allow-only should not be mutated")
	assert.Equal(t, allowOnlyBefore, allowOnly, "original allow-only should be unchanged")
}

// TestBuildLabelAgentPayload_MultipleBots tests injection of multiple bots.
func TestBuildLabelAgentPayload_MultipleBots(t *testing.T) {
	policy := basePolicy()
	trustedBots := []string{"dependabot[bot]", "renovate[bot]", "github-actions[bot]"}

	result := BuildLabelAgentPayload(policy, trustedBots, nil)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	bots, _ := resultMap["trusted-bots"].([]interface{})
	require.Len(t, bots, 3)
	assert.Equal(t, "dependabot[bot]", bots[0])
	assert.Equal(t, "renovate[bot]", bots[1])
	assert.Equal(t, "github-actions[bot]", bots[2])
}

// TestBuildLabelAgentPayload_MultipleUsers tests injection of multiple trusted users.
func TestBuildLabelAgentPayload_MultipleUsers(t *testing.T) {
	policy := basePolicy()
	trustedUsers := []string{"alice", "bob", "charlie"}

	result := BuildLabelAgentPayload(policy, nil, trustedUsers)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	allowOnly, _ := resultMap["allow-only"].(map[string]interface{})
	users, _ := allowOnly["trusted-users"].([]interface{})
	require.Len(t, users, 3)
	assert.Equal(t, "alice", users[0])
	assert.Equal(t, "bob", users[1])
	assert.Equal(t, "charlie", users[2])
}

// TestBuildLabelAgentPayload_AllReposPolicy tests with repos="all" policy.
func TestBuildLabelAgentPayload_AllReposPolicy(t *testing.T) {
	policy := map[string]interface{}{
		"allow-only": map[string]interface{}{
			"repos":         "all",
			"min-integrity": "approved",
		},
	}
	trustedBots := []string{"dependabot[bot]"}
	trustedUsers := []string{"alice"}

	result := BuildLabelAgentPayload(policy, trustedBots, trustedUsers)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	// Both should be injected
	_, hasBots := resultMap["trusted-bots"]
	assert.True(t, hasBots)

	allowOnly, _ := resultMap["allow-only"].(map[string]interface{})
	_, hasUsers := allowOnly["trusted-users"]
	assert.True(t, hasUsers)
}

// TestBuildLabelAgentPayload_NilPolicyWithBots tests that a nil policy is
// returned as-is when bots/users are provided, since GuardPolicyToMap will
// fail on nil and the fallback returns the original policy.
func TestBuildLabelAgentPayload_NilPolicyWithBots(t *testing.T) {
	result := BuildLabelAgentPayload(nil, []string{"bot"}, nil)
	assert.Nil(t, result, "nil policy should be returned as-is on conversion error")
}

// TestBuildLabelAgentPayload_InvalidPolicyWithBots tests that an unserializable
// policy (e.g. a channel) is returned as-is.
func TestBuildLabelAgentPayload_InvalidPolicyWithBots(t *testing.T) {
	ch := make(chan int)
	result := BuildLabelAgentPayload(ch, []string{"bot"}, nil)
	assert.Equal(t, ch, result, "unserializable policy should be returned as-is")
}

// TestBuildLabelAgentPayload_ExistingTrustedBotsPreserved tests that if the policy
// already has trusted-bots, the injected list replaces/overwrites it.
func TestBuildLabelAgentPayload_ExistingTrustedBotsOverwritten(t *testing.T) {
	policy := map[string]interface{}{
		"allow-only": map[string]interface{}{
			"repos":         "public",
			"min-integrity": "none",
		},
		"trusted-bots": []interface{}{"existing-bot"},
	}
	newBots := []string{"new-bot"}

	result := BuildLabelAgentPayload(policy, newBots, nil)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	bots, _ := resultMap["trusted-bots"].([]interface{})
	require.Len(t, bots, 1)
	assert.Equal(t, "new-bot", bots[0], "new trusted-bots should overwrite existing")
}
