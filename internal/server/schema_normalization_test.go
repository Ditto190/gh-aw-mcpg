package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/mcp"
)

// TestSchemaNormalization_Integration tests that broken schemas from backends
// are automatically normalized when tools are registered.
func TestSchemaNormalization_Integration(t *testing.T) {
	ctx := context.Background()

	// Create a minimal config
	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			"test": {
				Command: "echo", // Dummy command
				Args:    []string{},
			},
		},
	}

	// Create unified server
	us, err := NewUnified(ctx, cfg)
	require.NoError(t, err, "Failed to create unified server")
	defer us.Close()

	testCases := []struct {
		name           string
		toolName       string
		inputSchema    map[string]interface{}
		expectedSchema map[string]interface{}
	}{
		{
			name:     "broken object schema without properties",
			toolName: "get_commit",
			inputSchema: map[string]interface{}{
				"type": "object",
			},
			expectedSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			name:     "valid object schema with properties",
			toolName: "issue_read",
			inputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"owner": map[string]interface{}{
						"type": "string",
					},
					"repo": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []interface{}{"owner", "repo"},
			},
			expectedSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"owner": map[string]interface{}{
						"type": "string",
					},
					"repo": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []interface{}{"owner", "repo"},
			},
		},
		{
			name:     "object schema with additionalProperties",
			toolName: "list_items",
			inputSchema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			expectedSchema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate registering a tool with the given schema
			prefixedName := "test___" + tc.toolName

			// Use the NormalizeInputSchema function directly
			normalized := mcp.NormalizeInputSchema(tc.inputSchema, prefixedName)

			// Store the normalized tool
			us.toolsMu.Lock()
			us.tools[prefixedName] = &ToolInfo{
				Name:        prefixedName,
				Description: "Test tool",
				BackendID:   "test",
				InputSchema: normalized,
			}
			us.toolsMu.Unlock()

			// Retrieve the tool and verify the schema
			us.toolsMu.RLock()
			tool, exists := us.tools[prefixedName]
			us.toolsMu.RUnlock()

			require.True(t, exists, "Tool should exist")
			assert.Equal(t, tc.expectedSchema, tool.InputSchema, "Schema should match expected normalized version")

			// Clean up
			us.toolsMu.Lock()
			delete(us.tools, prefixedName)
			us.toolsMu.Unlock()
		})
	}
}

// TestSchemaNormalization_PreservesOriginal verifies that the normalization
// doesn't modify the original schema object
func TestSchemaNormalization_PreservesOriginal(t *testing.T) {
	original := map[string]interface{}{
		"type": "object",
	}

	// Make a copy to compare later
	originalCopy := make(map[string]interface{})
	for k, v := range original {
		originalCopy[k] = v
	}

	// Normalize the schema
	normalized := mcp.NormalizeInputSchema(original, "test-tool")

	// Verify original is unchanged
	assert.Equal(t, originalCopy, original, "Original schema should not be modified")

	// Verify normalized has properties
	_, hasProperties := normalized["properties"]
	assert.True(t, hasProperties, "Normalized schema should have properties")

	// Verify original still doesn't have properties
	_, originalHasProperties := original["properties"]
	assert.False(t, originalHasProperties, "Original schema should not have properties")
}

// TestNormalizeInputSchema tests all branches of the NormalizeInputSchema function.
func TestNormalizeInputSchema(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:   "nil schema returns default empty object schema",
			schema: nil,
			expected: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			name:   "empty schema (no type, no properties) returns empty object schema",
			schema: map[string]interface{}{},
			expected: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			name: "schema without type but with properties adds object type",
			schema: map[string]interface{}{
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
				},
			},
			expected: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
				},
			},
		},
		{
			name: "string type schema returned as-is",
			schema: map[string]interface{}{
				"type": "string",
			},
			expected: map[string]interface{}{
				"type": "string",
			},
		},
		{
			name: "array type schema returned as-is",
			schema: map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
			expected: map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
			},
		},
		{
			name: "non-string type value returned as-is",
			schema: map[string]interface{}{
				"type": 42,
			},
			expected: map[string]interface{}{
				"type": 42,
			},
		},
		{
			name: "object with properties and additionalProperties returned as-is",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
				"additionalProperties": true,
			},
			expected: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
				"additionalProperties": true,
			},
		},
		{
			name: "object with only additionalProperties returned as-is",
			schema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
			},
			expected: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mcp.NormalizeInputSchema(tt.schema, "test-tool")
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNormalizeInputSchema_PreservesOriginalForAllMutatingBranches verifies that
// the normalization never modifies the original schema for any code path that
// creates a copy (no-type-with-properties and object-without-properties branches).
func TestNormalizeInputSchema_PreservesOriginalForAllMutatingBranches(t *testing.T) {
	t.Run("no-type schema with properties is not mutated", func(t *testing.T) {
		original := map[string]interface{}{
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
			},
		}
		_, hadType := original["type"]

		mcp.NormalizeInputSchema(original, "test-tool")

		_, nowHasType := original["type"]
		assert.Equal(t, hadType, nowHasType, "Original schema should not gain a type field")
	})

	t.Run("object schema without properties is not mutated", func(t *testing.T) {
		original := map[string]interface{}{
			"type": "object",
		}
		_, hadProperties := original["properties"]

		mcp.NormalizeInputSchema(original, "test-tool")

		_, nowHasProperties := original["properties"]
		assert.Equal(t, hadProperties, nowHasProperties, "Original schema should not gain a properties field")
	})
}
