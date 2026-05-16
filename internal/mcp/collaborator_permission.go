package mcp

import (
	"fmt"
)

// ParseCollaboratorPermissionArgs extracts and validates the owner, repo, and
// username fields from an args map for a get_collaborator_permission call.
// It returns the (possibly partial) values even on error so that callers can
// include them in diagnostic log messages.
func ParseCollaboratorPermissionArgs(argsMap map[string]interface{}) (owner, repo, username string, err error) {
	owner, _ = argsMap["owner"].(string)
	repo, _ = argsMap["repo"].(string)
	username, _ = argsMap["username"].(string)
	if owner == "" || repo == "" || username == "" {
		err = fmt.Errorf("get_collaborator_permission: missing owner/repo/username")
	}
	return
}
