package delegation

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/util"
)

func captureControlHandlerLogs(t *testing.T, f func()) string {
	t.Helper()
	t.Setenv("DEBUG", "*")
	t.Setenv("DEBUG_COLORS", "0")

	previous := logControlHandler
	logControlHandler = logger.New("delegation:control_handler")
	require.True(t, logControlHandler.Enabled(), "the capture harness must actually enable debug logging")
	t.Cleanup(func() { logControlHandler = previous })

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

func TestHandleControl_CreateOrConfirmSuccessLogging(t *testing.T) {
	wire := CreateOrConfirmRequestWire{
		RunID:               "run-123",
		EnclaveBackend:      "awf-enclave",
		EnclaveEntryID:      "entry-1",
		InvocationID:        "inv-1",
		Repository:          "github/gh-aw",
		ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
		SchemaHash:          "sha256:abc",
		RequestedTTLSeconds: 60,
		IdempotencyKey:      "idem-1",
	}

	t.Run("logs success with invocation hash after persistence succeeds", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		var responseStatus int
		logs := captureControlHandlerLogs(t, func() {
			responseStatus = doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire).Code
		})

		require.Equal(t, http.StatusOK, responseStatus)
		assert.Contains(t, logs, "create-or-confirm succeeded:")
		assert.Contains(t, logs, "invocation_id_hash="+util.HashForLog(wire.InvocationID, 16, ""))
	})

	t.Run("does not log success when persistence fails", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		badDir := filepath.Join(t.TempDir(), "not-a-file")
		require.NoError(t, os.MkdirAll(badDir, 0o755))
		deps.StatePath = badDir

		var responseStatus int
		logs := captureControlHandlerLogs(t, func() {
			responseStatus = doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire).Code
		})

		require.Equal(t, http.StatusInternalServerError, responseStatus)
		assert.NotContains(t, logs, "create-or-confirm succeeded:")
	})
}
