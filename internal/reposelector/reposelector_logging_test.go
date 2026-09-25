package reposelector

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

func captureReposelectorLogs(t *testing.T, f func()) string {
	t.Helper()
	t.Setenv("DEBUG", "*")
	t.Setenv("DEBUG_COLORS", "0")

	previous := logReposelector
	logReposelector = logger.New("reposelector:reposelector")
	require.True(t, logReposelector.Enabled(), "the capture harness must actually enable debug logging")
	t.Cleanup(func() { logReposelector = previous })

	original := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	var (
		wg  sync.WaitGroup
		buf bytes.Buffer
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, r)
	}()

	func() {
		defer func() {
			os.Stderr = original
			_ = w.Close()
		}()
		f()
	}()
	wg.Wait()
	_ = r.Close()
	return buf.String()
}

func TestIsCanonicalRepositorySelectorLogsRejectionReason(t *testing.T) {
	tests := []struct {
		name      string
		selector  string
		wantLog   string
		wantValid bool
	}{
		{"non-ASCII byte", "gi™thub/gh-aw", "non-ASCII bytes present", false},
		{"pattern mismatch", "GitHub/gh-aw", "does not match canonical owner/repo pattern", false},
		{"repo segment is dot-dot", "github/..", "repo segment is '.', '..', or contains '..'", false},
		{"repo segment is dot", "github/.", "repo segment is '.', '..', or contains '..'", false},
		{"repo segment contains dot-dot", "github/foo..bar", "repo segment is '.', '..', or contains '..'", false},
		{"canonical selector logs nothing", "github/gh-aw", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			logs := captureReposelectorLogs(t, func() {
				got = IsCanonicalRepositorySelector(tt.selector)
			})
			assert.Equal(t, tt.wantValid, got, "selector %q", tt.selector)
			if tt.wantLog == "" {
				assert.Empty(t, logs, "a canonical selector must not log a rejection")
				return
			}
			assert.Contains(t, logs, tt.wantLog, "selector %q", tt.selector)
		})
	}
}

func TestIsCanonicalOwnerLogsRejectionReason(t *testing.T) {
	tests := []struct {
		name      string
		owner     string
		wantLog   string
		wantValid bool
	}{
		{"uppercase is rejected and logged", "GitHub", "not a canonical ASCII owner segment", false},
		{"non-ASCII byte is rejected and logged", "gi™thub", "not a canonical ASCII owner segment", false},
		{"canonical owner logs nothing", "github", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			logs := captureReposelectorLogs(t, func() {
				got = IsCanonicalOwner(tt.owner)
			})
			assert.Equal(t, tt.wantValid, got, "owner %q", tt.owner)
			if tt.wantLog == "" {
				assert.Empty(t, logs, "a canonical owner must not log a rejection")
				return
			}
			assert.Contains(t, logs, tt.wantLog, "owner %q", tt.owner)
		})
	}
}
