package util

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to filePath by syncing a uniquely named
// temporary file in the destination directory before renaming it into place.
func AtomicWriteFile(filePath string, data []byte, perm os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(filePath), "."+filepath.Base(filePath)+"-*")
	if err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		return fmt.Errorf("failed to set temp file permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}
