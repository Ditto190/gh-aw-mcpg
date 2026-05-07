package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultWasmCacheDir(t *testing.T) {
	assert.Equal(t, "/tmp/logs/"+config.DefaultWasmCacheDirName, defaultWasmCacheDir("/tmp/logs"))
}

func TestResolveWasmCacheDir(t *testing.T) {
	t.Run("defaults beneath log dir when no override is set", func(t *testing.T) {
		t.Setenv(wasmCacheDirEnvVar, "")
		assert.Equal(t, defaultWasmCacheDir("/tmp/logs"), resolveWasmCacheDir(false, "", "/tmp/logs"))
	})

	t.Run("environment override is used when flag is unchanged", func(t *testing.T) {
		t.Setenv(wasmCacheDirEnvVar, "/tmp/custom-cache")
		assert.Equal(t, "/tmp/custom-cache", resolveWasmCacheDir(false, "", "/tmp/logs"))
	})

	t.Run("flag override takes precedence over environment", func(t *testing.T) {
		t.Setenv(wasmCacheDirEnvVar, "/tmp/custom-cache")
		assert.Equal(t, "/tmp/flag-cache", resolveWasmCacheDir(true, "/tmp/flag-cache", "/tmp/logs"))
	})

	t.Run("blank flag falls back to environment or default", func(t *testing.T) {
		t.Setenv(wasmCacheDirEnvVar, "/tmp/custom-cache")
		assert.Equal(t, "/tmp/custom-cache", resolveWasmCacheDir(true, "   ", "/tmp/logs"))
	})
}

func TestConfigureWasmCompilationCache(t *testing.T) {
	t.Run("falls back to in-memory cache when disk cache init fails", func(t *testing.T) {
		ctx := context.Background()
		tempFile, err := os.CreateTemp(t.TempDir(), "not-a-dir")
		require.NoError(t, err)
		require.NoError(t, tempFile.Close())

		warnings := make([]string, 0, 1)
		dir, err := configureWasmCompilationCache(ctx, true, tempFile.Name(), "/tmp/logs", func(format string, args ...interface{}) {
			warnings = append(warnings, format)
		})
		require.NoError(t, err)
		assert.Empty(t, dir)
		assert.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "Falling back to in-memory WASM compilation cache")

		t.Cleanup(func() {
			require.NoError(t, guard.ConfigureGlobalCompilationCache(ctx, ""))
		})
	})
}
