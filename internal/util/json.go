package util

// DeepCloneJSON creates a deep copy of a JSON-compatible value.
// It handles the three container types used by encoding/json:
// map[string]any (JSON objects), []any (JSON arrays),
// and any other type (JSON scalars: string, float64, bool, nil), which is
// returned as-is since scalar values are not reference types and need no cloning.
func DeepCloneJSON(v any) any {
	switch val := v.(type) {
	case map[string]any:
		clone := make(map[string]any, len(val))
		for k, v := range val {
			clone[k] = DeepCloneJSON(v)
		}
		return clone
	case []any:
		clone := make([]any, len(val))
		for i, v := range val {
			clone[i] = DeepCloneJSON(v)
		}
		return clone
	default:
		return v
	}
}
