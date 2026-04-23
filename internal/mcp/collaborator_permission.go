package mcp

import "encoding/json"

// LogAndWrapCollaboratorPermission parses the raw GitHub API response body for a
// get_collaborator_permission request, logs the resolved permission level for
// observability, and returns the body wrapped in MCP text-response format.
//
// This helper is shared between the server and proxy packages to eliminate
// duplicated parse/log/wrap logic. Callers pass their own debug logger's Printf
// method so that log lines appear under the correct namespace.
func LogAndWrapCollaboratorPermission(
	body []byte,
	owner, repo, username string,
	statusCode int,
	logPrintf func(format string, args ...interface{}),
) interface{} {
	var permResp map[string]interface{}
	if jsonErr := json.Unmarshal(body, &permResp); jsonErr == nil {
		if perm, ok := permResp["permission"].(string); ok {
			logPrintf("get_collaborator_permission: %s/%s user %s → permission=%q (HTTP %d)", owner, repo, username, perm, statusCode)
		} else {
			logPrintf("get_collaborator_permission: %s/%s user %s → HTTP %d, permission field missing from response", owner, repo, username, statusCode)
		}
	} else {
		logPrintf("get_collaborator_permission: %s/%s user %s → HTTP %d, %d bytes (JSON parse failed: %v)", owner, repo, username, statusCode, len(body), jsonErr)
	}
	return BuildMCPTextResponse(string(body))
}
