package config

// IsStdioServerType reports whether t is a stdio-family server type.
// Empty type, "stdio", and the "local" alias all map to stdio.
func IsStdioServerType(t string) bool {
	result := t == "" || t == "stdio" || t == "local"
	logConfig.Printf("IsStdioServerType: type=%q, result=%v", t, result)
	return result
}

// NormalizeServerType maps legacy/empty type strings to canonical values.
// "" and "local" normalize to "stdio"; all other values are returned unchanged.
func NormalizeServerType(t string) string {
	if t == "" || t == "local" {
		logConfig.Printf("NormalizeServerType: normalizing %q to \"stdio\"", t)
		return "stdio"
	}
	logConfig.Printf("NormalizeServerType: type %q unchanged", t)
	return t
}
