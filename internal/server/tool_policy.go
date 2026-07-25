package server

import "strings"

// IsSingularReadTool returns true when toolName refers to a tool expected to
// return a single resource (e.g. get_*, *_read). List/search tools are treated
// as collection tools even if they happen to return one item.
//
// This function encodes server-layer policy about how GitHub-specific MCP tool
// results should be handled in DIFC filtering; it lives here rather than in the
// mcp (wire protocol) package because it is a server-level policy decision, not
// a protocol-level concern.
func IsSingularReadTool(toolName string) bool {
	return !strings.HasPrefix(toolName, "list_") && !strings.HasPrefix(toolName, "search_")
}
