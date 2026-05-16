package fileutil

import (
	"os"
	"path/filepath"
)

// FileExists checks if the file or directory exists at the given path
func FileExists(path string) bool {
	fullPath := filepath.Join(path)
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
