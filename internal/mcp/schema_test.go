package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeInputSchema_NilSchema(t *testing.T) {
	result := NormalizeInputSchema(nil, "my-tool")

	assert.Equal(t, map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}, result)
}

func TestNormalizeInputSchema_AllBranches(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]interface{}
		expected map[string]interface{}
	}{
		{
			// No type, no properties → empty object schema
			name:   "empty schema returns default empty object schema",
			schema: map[string]interface{}{},
			expected: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			// No type, has properties → add type "object", preserve all fields
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
			// Non-"object" type string → returned as-is
			name: "string type schema returned as-is",
			schema: map[string]interface{}{
				"type": "string",
			},
			expected: map[string]interface{}{
				"type": "string",
			},
		},
		{
			// Non-"object" type string with items → returned as-is
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
			// Non-string type value → returned as-is
			name: "non-string type value returned as-is",
			schema: map[string]interface{}{
				"type": 42,
			},
			expected: map[string]interface{}{
				"type": 42,
			},
		},
		{
			// Object type, has properties → returned as-is
			name: "object schema with properties returned as-is",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"owner": map[string]interface{}{"type": "string"},
				},
			},
			expected: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"owner": map[string]interface{}{"type": "string"},
				},
			},
		},
		{
			// Object type, only additionalProperties (no properties key) → returned as-is
			name: "object schema with only additionalProperties returned as-is",
			schema: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			expected: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
		{
			// Object type, both properties and additionalProperties → returned as-is
			name: "object schema with properties and additionalProperties returned as-is",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
				"additionalProperties": false,
			},
			expected: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
				"additionalProperties": false,
			},
		},
		{
			// Object type, missing both properties and additionalProperties → add empty properties
			name: "object type without properties gets empty properties added",
			schema: map[string]interface{}{
				"type": "object",
			},
			expected: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeInputSchema(tt.schema, "test-tool")
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNormalizeInputSchema_PreservesExtraFields verifies that extra fields like
// "required", "description", and "$schema" are preserved in copy paths.
func TestNormalizeInputSchema_PreservesExtraFields(t *testing.T) {
	t.Run("no-type schema with properties preserves required and description", func(t *testing.T) {
		schema := map[string]interface{}{
			"description": "A tool that does things",
			"properties": map[string]interface{}{
				"owner": map[string]interface{}{"type": "string"},
				"repo":  map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"owner", "repo"},
		}

		result := NormalizeInputSchema(schema, "test-tool")

		assert.Equal(t, "object", result["type"])
		assert.Equal(t, "A tool that does things", result["description"])
		assert.Equal(t, []interface{}{"owner", "repo"}, result["required"])
		assert.NotNil(t, result["properties"])
	})

	t.Run("object schema without properties preserves required and description", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":        "object",
			"description": "Schema with required but no properties",
			"required":    []interface{}{"name"},
		}

		result := NormalizeInputSchema(schema, "test-tool")

		assert.Equal(t, "object", result["type"])
		assert.Equal(t, "Schema with required but no properties", result["description"])
		assert.Equal(t, []interface{}{"name"}, result["required"])
		assert.Equal(t, map[string]interface{}{}, result["properties"])
	})
}

// TestNormalizeInputSchema_DoesNotMutateOriginal verifies that the original schema
// is never modified, only a copy is returned for mutating branches.
func TestNormalizeInputSchema_DoesNotMutateOriginal(t *testing.T) {
	t.Run("nil schema does not panic and returns new map", func(t *testing.T) {
		result := NormalizeInputSchema(nil, "tool")
		assert.NotNil(t, result)
	})

	t.Run("no-type schema with properties is not mutated", func(t *testing.T) {
		original := map[string]interface{}{
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string"},
			},
		}
		_, hadType := original["type"]

		NormalizeInputSchema(original, "test-tool")

		_, nowHasType := original["type"]
		assert.Equal(t, hadType, nowHasType, "original should not gain a type field")
		assert.Len(t, original, 1, "original should still have only one key")
	})

	t.Run("object schema without properties is not mutated", func(t *testing.T) {
		original := map[string]interface{}{
			"type": "object",
		}
		_, hadProperties := original["properties"]

		NormalizeInputSchema(original, "test-tool")

		_, nowHasProperties := original["properties"]
		assert.Equal(t, hadProperties, nowHasProperties, "original should not gain a properties field")
		assert.Len(t, original, 1, "original should still have only one key")
	})

	t.Run("returned map is a different reference than original for mutating branches", func(t *testing.T) {
		original := map[string]interface{}{
			"type": "object",
		}

		result := NormalizeInputSchema(original, "test-tool")

		// Verify result is a distinct map (modifying result should not affect original)
		result["extra"] = "value"
		_, hasExtra := original["extra"]
		assert.False(t, hasExtra, "modifying the result should not affect the original")
	})
}

// TestNormalizeInputSchema_ToolNameVariants verifies that the toolName parameter
// is accepted in various forms without affecting normalization logic.
func TestNormalizeInputSchema_ToolNameVariants(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
	}
	expected := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	toolNames := []string{
		"simple",
		"",
		"server___tool-name",
		"tool with spaces",
		"tool/with/slashes",
		"very-long-tool-name-that-exceeds-normal-length-limits-for-mcp-tool-names",
	}

	for _, name := range toolNames {
		t.Run("tool name: "+name, func(t *testing.T) {
			result := NormalizeInputSchema(schema, name)
			assert.Equal(t, expected, result, "normalization result should not depend on toolName")
		})
	}
}

// TestNormalizeInputSchema_DeepNestedSchema verifies that deeply nested
// schemas are handled correctly without modification.
func TestNormalizeInputSchema_DeepNestedSchema(t *testing.T) {
	deep := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"nested": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"deeper": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
		},
		"required": []interface{}{"nested"},
	}

	result := NormalizeInputSchema(deep, "deep-tool")

	// Schema already valid, should be returned as-is
	assert.Equal(t, deep, result)
}
