package strutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepCloneJSON(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		assert.Nil(t, DeepCloneJSON(nil))
	})

	t.Run("scalar types are returned as-is", func(t *testing.T) {
		tests := []struct {
			name  string
			input interface{}
		}{
			{"string", "hello"},
			{"empty string", ""},
			{"integer", 42},
			{"float64", 3.14},
			{"bool true", true},
			{"bool false", false},
			{"zero int", 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.input, DeepCloneJSON(tt.input))
			})
		}
	})

	t.Run("empty map returns new empty map", func(t *testing.T) {
		input := map[string]interface{}{}
		result := DeepCloneJSON(input)
		require.NotNil(t, result)
		assert.Equal(t, map[string]interface{}{}, result)
	})

	t.Run("empty slice returns new empty slice", func(t *testing.T) {
		input := []interface{}{}
		result := DeepCloneJSON(input)
		require.NotNil(t, result)
		assert.Equal(t, []interface{}{}, result)
	})

	t.Run("flat map is cloned correctly", func(t *testing.T) {
		input := map[string]interface{}{
			"name":   "alice",
			"age":    float64(30),
			"active": true,
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)
	})

	t.Run("flat slice is cloned correctly", func(t *testing.T) {
		input := []interface{}{"a", "b", "c"}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)
	})

	t.Run("map with nested map is deep cloned", func(t *testing.T) {
		input := map[string]interface{}{
			"outer": map[string]interface{}{
				"inner": "value",
			},
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)

		// Mutating the nested clone does not affect original
		clone := result.(map[string]interface{})
		clone["outer"].(map[string]interface{})["inner"] = "modified"
		assert.Equal(t, "value", input["outer"].(map[string]interface{})["inner"],
			"original nested map should not be affected by clone mutation")
	})

	t.Run("map with slice value is deep cloned", func(t *testing.T) {
		input := map[string]interface{}{
			"items": []interface{}{"x", "y"},
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)

		// Mutating the cloned slice does not affect original
		clone := result.(map[string]interface{})
		clone["items"].([]interface{})[0] = "modified"
		assert.Equal(t, "x", input["items"].([]interface{})[0],
			"original slice should not be affected by clone mutation")
	})

	t.Run("slice of maps is deep cloned", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{"key": "val1"},
			map[string]interface{}{"key": "val2"},
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)

		// Mutating a map inside the cloned slice does not affect original
		clone := result.([]interface{})
		clone[0].(map[string]interface{})["key"] = "modified"
		assert.Equal(t, "val1", input[0].(map[string]interface{})["key"],
			"original map in slice should not be affected by clone mutation")
	})

	t.Run("slice of slices is deep cloned", func(t *testing.T) {
		input := []interface{}{
			[]interface{}{1, 2},
			[]interface{}{3, 4},
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)

		// Mutating a nested slice in the clone does not affect original
		clone := result.([]interface{})
		clone[0].([]interface{})[0] = 99
		assert.Equal(t, 1, input[0].([]interface{})[0],
			"original nested slice should not be affected by clone mutation")
	})

	t.Run("deeply nested structure is fully cloned", func(t *testing.T) {
		input := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": "deep-value",
				},
			},
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)

		// Mutate deep level of clone, original must be unchanged
		clone := result.(map[string]interface{})
		l2 := clone["level1"].(map[string]interface{})
		l3 := l2["level2"].(map[string]interface{})
		l3["level3"] = "mutated"

		origL2 := input["level1"].(map[string]interface{})
		origL3 := origL2["level2"].(map[string]interface{})
		assert.Equal(t, "deep-value", origL3["level3"],
			"original deep value should not be affected")
	})

	t.Run("cloned map is independent: original mutation does not affect clone", func(t *testing.T) {
		input := map[string]interface{}{"k": "v"}
		clone := DeepCloneJSON(input)
		input["k"] = "changed"
		assert.Equal(t, "v", clone.(map[string]interface{})["k"],
			"clone should not reflect changes to original")
	})

	t.Run("cloned slice is independent: original mutation does not affect clone", func(t *testing.T) {
		input := []interface{}{"a", "b"}
		clone := DeepCloneJSON(input)
		input[0] = "changed"
		assert.Equal(t, "a", clone.([]interface{})[0],
			"clone should not reflect changes to original slice")
	})

	t.Run("map with nil value is cloned correctly", func(t *testing.T) {
		input := map[string]interface{}{
			"key":   "value",
			"empty": nil,
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)
		assert.Nil(t, result.(map[string]interface{})["empty"])
	})

	t.Run("mixed types in map are preserved", func(t *testing.T) {
		input := map[string]interface{}{
			"str":    "hello",
			"num":    float64(42),
			"bool":   true,
			"null":   nil,
			"nested": map[string]interface{}{"x": 1},
			"list":   []interface{}{1, 2, 3},
		}
		result := DeepCloneJSON(input)
		assert.Equal(t, input, result)
	})

	t.Run("cloned map has correct length", func(t *testing.T) {
		input := map[string]interface{}{"a": 1, "b": 2, "c": 3}
		result := DeepCloneJSON(input)
		clone := result.(map[string]interface{})
		assert.Len(t, clone, 3)
	})

	t.Run("cloned slice has correct length", func(t *testing.T) {
		input := []interface{}{10, 20, 30, 40}
		result := DeepCloneJSON(input)
		clone := result.([]interface{})
		assert.Len(t, clone, 4)
	})
}
