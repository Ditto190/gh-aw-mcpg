package guard

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWasmGuardLogsPayloadSizeWithoutResponseBody(t *testing.T) {
	t.Setenv("DEBUG", "*")
	require.NoError(t, logger.CloseAllLoggers())
	logDir := t.TempDir()
	logger.InitProxyLoggers(logDir)
	t.Cleanup(func() {
		require.NoError(t, logger.CloseAllLoggers())
	})

	guard, cleanup := setupRawWasmModule(t, labelResponseWritesEmptyObjectWasm, "secret-log-test")
	defer cleanup()
	const privateBodyMarker = "PRIVATE_RESPONSE_BODY_MUST_NOT_BE_LOGGED"
	_, err := guard.LabelResponse(
		context.Background(),
		"issue_read",
		map[string]interface{}{"body": privateBodyMarker},
		nil,
		difc.NewCapabilities(),
	)
	require.NoError(t, err)
	require.NoError(t, logger.CloseAllLoggers())

	var combined strings.Builder
	require.NoError(t, filepath.WalkDir(logDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		combined.Write(content)
		return nil
	}))
	assert.NotContains(t, combined.String(), privateBodyMarker)
}
