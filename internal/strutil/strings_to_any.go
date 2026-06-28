package strutil

// StringsToAny converts a []string to []interface{}.
func StringsToAny(input []string) []interface{} {
	out := make([]interface{}, len(input))
	for i, value := range input {
		out[i] = value
	}
	return out
}
