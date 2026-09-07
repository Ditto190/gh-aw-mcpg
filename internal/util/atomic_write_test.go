package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicWriteFile(t *testing.T) {
	t.Run("writes content with requested permissions", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		data := []byte(`{"key":"value"}`)

		require.NoError(t, AtomicWriteFile(path, data, 0o600))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, data, got)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		assert.Empty(t, tempFiles(t, filepath.Dir(path)))
	})

	t.Run("fails when parent directory does not exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "state.json")

		err := AtomicWriteFile(path, []byte("data"), 0o600)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write temp file")
	})

	t.Run("cleans up when rename fails", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, os.Mkdir(path, 0o755))

		err := AtomicWriteFile(path, []byte("data"), 0o600)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to rename temp file")
		assert.Empty(t, tempFiles(t, filepath.Dir(path)))
	})
}

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.Type().IsRegular() && len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			files = append(files, entry.Name())
		}
	}
	return files
}
