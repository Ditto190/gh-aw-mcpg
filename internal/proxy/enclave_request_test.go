package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/enclavegithub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanEnclaveRequest(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		target       string
		body         string
		wantDenial   enclaveDenial
		wantPath     string
		wantFullPath string
		wantTool     string
	}{
		{
			name:         "issues list",
			method:       http.MethodGet,
			target:       "/repos/owner/repo/issues?state=open",
			wantDenial:   enclaveDenialNone,
			wantPath:     "/repos/owner/repo/issues",
			wantFullPath: "/repos/owner/repo/issues?state=open",
			wantTool:     "list_issues",
		},
		{
			name:         "issue get without query",
			method:       http.MethodGet,
			target:       "/repos/owner/repo/issues/12",
			wantDenial:   enclaveDenialNone,
			wantPath:     "/repos/owner/repo/issues/12",
			wantFullPath: "/repos/owner/repo/issues/12",
			wantTool:     "issue_read",
		},
		{
			name:       "non GET method denied",
			method:     http.MethodPost,
			target:     "/repos/owner/repo/issues",
			wantDenial: enclaveDenialRequestShape,
			wantPath:   "/repos/owner/repo/issues",
		},
		{
			name:       "GET with body denied",
			method:     http.MethodGet,
			target:     "/repos/owner/repo/issues",
			body:       "{}",
			wantDenial: enclaveDenialRequestShape,
			wantPath:   "/repos/owner/repo/issues",
		},
		{
			name:       "unmatched route denied",
			method:     http.MethodGet,
			target:     "/repos/owner/repo/pulls",
			wantDenial: enclaveDenialRoute,
			wantPath:   "/repos/owner/repo/pulls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.target, body)

			plan := planEnclaveRequest(req)

			assert.Equal(t, tt.wantDenial, plan.denial)
			assert.Equal(t, tt.wantDenial == enclaveDenialNone, plan.ok())
			assert.Equal(t, tt.wantPath, plan.path)
			assert.Equal(t, tt.wantFullPath, plan.fullPath)
			assert.Equal(t, tt.wantTool, plan.toolName)
			if tt.wantDenial == enclaveDenialNone {
				require.NotNil(t, plan.route)
				assert.Equal(t, "owner/repo", plan.route.FullRepo())
				assert.NotEmpty(t, plan.args)
			}
		})
	}
}

func TestPlanEnclaveRequestInvalidPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/repos/owner/repo/issues", nil)
	req.URL.Path = ghHostPathPrefix

	plan := planEnclaveRequest(req)

	assert.Equal(t, enclaveDenialPath, plan.denial)
	assert.False(t, plan.ok())
	assert.Empty(t, plan.path)
}

func TestPlanEnclaveRequestStripsHostPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, ghHostPathPrefix+"/repos/owner/repo/issues/7/comments", nil)

	plan := planEnclaveRequest(req)

	require.True(t, plan.ok())
	assert.Equal(t, "/repos/owner/repo/issues/7/comments", plan.path)
	assert.Equal(t, "issue_read", plan.toolName)
	require.NotNil(t, plan.route)
	assert.Equal(t, enclavegithub.OperationIssueCommentsList, plan.route.Operation)
}

func TestEnclaveToolAndArgsUnsupportedOperation(t *testing.T) {
	// enclaveToolAndArgs is the tool-resolution step of planEnclaveRequest; an
	// operation with no backing MCP tool yields an empty tool name, which the
	// planner turns into an enclaveDenialTool denial.
	toolName, args := enclaveToolAndArgs(&enclavegithub.Route{Operation: "issues.delete", Owner: "owner", Repo: "repo"})

	assert.Empty(t, toolName)
	assert.Nil(t, args)
}
