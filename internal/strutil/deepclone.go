package strutil

// DeepCloneJSON creates a deep copy of a JSON-compatible value.
// It handles the three container types used by encoding/json:
// map[string]interface{} (JSON objects), []interface{} (JSON arrays),
// and any other type (JSON scalars: string, float64, bool, nil), which is
// returned as-is since scalar values are not reference types and need no cloning.
func DeepCloneJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		clone := make(map[string]interface{}, len(val))
		for k, v := range val {
			clone[k] = DeepCloneJSON(v)
		}
		return clone
	case []interface{}:
		clone := make([]interface{}, len(val))
		for i, v := range val {
			clone[i] = DeepCloneJSON(v)
		}
		return clone
	default:
		return v
	}
}
