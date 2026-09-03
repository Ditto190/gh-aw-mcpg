package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepCloneJSON(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns nil", func(t *testing.T) {
		t.Parallel()
		result := DeepCloneJSON(nil)
		assert.Nil(t, result)
	})

	t.Run("string returns same value", func(t *testing.T) {
		t.Parallel()
		result := DeepCloneJSON("hello")
		assert.Equal(t, "hello", result)
	})

	t.Run("float64 returns same value", func(t *testing.T) {
		t.Parallel()
		result := DeepCloneJSON(float64(3.14))
		assert.InEpsilon(t, 3.14, result, 1e-9)
	})

	t.Run("bool true returns same value", func(t *testing.T) {
		t.Parallel()
		result := DeepCloneJSON(true)
		cloned, ok := result.(bool)
		require.True(t, ok, "result should be bool")
		assert.True(t, cloned)
	})

	t.Run("bool false returns same value", func(t *testing.T) {
		t.Parallel()
		result := DeepCloneJSON(false)
		cloned, ok := result.(bool)
		require.True(t, ok, "result should be bool")
		assert.False(t, cloned)
	})

	t.Run("empty map returns empty map", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok, "result should be map[string]any")
		assert.Empty(t, cloned)
	})

	t.Run("flat map with primitive values", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"name":   "alice",
			"age":    float64(30),
			"active": true,
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok, "result should be map[string]any")
		assert.Equal(t, input, cloned)
	})

	t.Run("flat map clone is independent from original", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"key": "original",
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok)

		cloned["key"] = "modified"
		assert.Equal(t, "original", input["key"], "original map should not be affected by clone modification")
	})

	t.Run("nested map deep clones nested maps", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"outer": map[string]any{
				"inner": "value",
			},
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, input, cloned)
	})

	t.Run("nested map clone is independent from original", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"outer": map[string]any{
				"inner": "original",
			},
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok)

		innerClone, ok := cloned["outer"].(map[string]any)
		require.True(t, ok)
		innerClone["inner"] = "modified"

		innerOrig := input["outer"].(map[string]any)
		assert.Equal(t, "original", innerOrig["inner"], "original nested map should not be affected")
	})

	t.Run("empty slice returns empty slice", func(t *testing.T) {
		t.Parallel()
		input := []any{}
		result := DeepCloneJSON(input)
		cloned, ok := result.([]any)
		require.True(t, ok, "result should be []any")
		assert.Empty(t, cloned)
	})

	t.Run("flat slice with primitive values", func(t *testing.T) {
		t.Parallel()
		input := []any{"a", float64(1), true, nil}
		result := DeepCloneJSON(input)
		cloned, ok := result.([]any)
		require.True(t, ok, "result should be []any")
		assert.Equal(t, input, cloned)
	})

	t.Run("flat slice clone is independent from original", func(t *testing.T) {
		t.Parallel()
		input := []any{"original", float64(42)}
		result := DeepCloneJSON(input)
		cloned, ok := result.([]any)
		require.True(t, ok)

		cloned[0] = "modified"
		assert.Equal(t, "original", input[0], "original slice should not be affected by clone modification")
	})

	t.Run("nested slice deep clones nested slices", func(t *testing.T) {
		t.Parallel()
		input := []any{
			[]any{"a", "b"},
			[]any{float64(1), float64(2)},
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.([]any)
		require.True(t, ok)
		assert.Equal(t, input, cloned)
	})

	t.Run("nested slice clone is independent from original", func(t *testing.T) {
		t.Parallel()
		input := []any{
			[]any{"original"},
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.([]any)
		require.True(t, ok)

		innerClone, ok := cloned[0].([]any)
		require.True(t, ok)
		innerClone[0] = "modified"

		innerOrig := input[0].([]any)
		assert.Equal(t, "original", innerOrig[0], "original nested slice should not be affected")
	})

	t.Run("map containing slices", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"items": []any{"x", "y", "z"},
			"count": float64(3),
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, input, cloned)

		// Verify independence of nested slice
		clonedItems, ok := cloned["items"].([]any)
		require.True(t, ok)
		clonedItems[0] = "modified"

		origItems := input["items"].([]any)
		assert.Equal(t, "x", origItems[0], "original slice inside map should not be affected")
	})

	t.Run("slice containing maps", func(t *testing.T) {
		t.Parallel()
		input := []any{
			map[string]any{"name": "alice", "score": float64(95)},
			map[string]any{"name": "bob", "score": float64(87)},
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.([]any)
		require.True(t, ok)
		assert.Equal(t, input, cloned)

		// Verify independence of nested map
		clonedMap, ok := cloned[0].(map[string]any)
		require.True(t, ok)
		clonedMap["name"] = "charlie"

		origMap := input[0].(map[string]any)
		assert.Equal(t, "alice", origMap["name"], "original map inside slice should not be affected")
	})

	t.Run("deeply nested structure", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": []any{
						map[string]any{
							"leaf": "value",
						},
					},
				},
			},
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, input, cloned)

		// Verify deep independence
		l1 := cloned["level1"].(map[string]any)
		l2 := l1["level2"].(map[string]any)
		l3 := l2["level3"].([]any)
		leaf := l3[0].(map[string]any)
		leaf["leaf"] = "modified"

		origL1 := input["level1"].(map[string]any)
		origL2 := origL1["level2"].(map[string]any)
		origL3 := origL2["level3"].([]any)
		origLeaf := origL3[0].(map[string]any)
		assert.Equal(t, "value", origLeaf["leaf"], "deeply nested original should not be affected")
	})

	t.Run("map with null value", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"key": nil,
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Nil(t, cloned["key"])
	})

	t.Run("slice with null element", func(t *testing.T) {
		t.Parallel()
		input := []any{nil, "value", nil}
		result := DeepCloneJSON(input)
		cloned, ok := result.([]any)
		require.True(t, ok)
		assert.Nil(t, cloned[0])
		assert.Equal(t, "value", cloned[1])
		assert.Nil(t, cloned[2])
	})

	t.Run("map preserves all key-value pairs", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"a": "alpha",
			"b": float64(2),
			"c": true,
			"d": nil,
			"e": []any{"x"},
			"f": map[string]any{"nested": "yes"},
		}
		result := DeepCloneJSON(input)
		cloned, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Len(t, cloned, len(input))
		assert.Equal(t, input, cloned)
	})
}
