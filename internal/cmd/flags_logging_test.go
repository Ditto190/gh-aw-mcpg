package cmd

import (
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/stretchr/testify/assert"
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
