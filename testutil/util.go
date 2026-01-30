package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// Writes files (specified as a map of {relative_path: file_content}) into the baseDir,
// creating subdirectories as needed.
func WriteFileTree(t *testing.T, baseDir string, files map[string]string) {
	for path, content := range files {
		fullPath := filepath.Join(baseDir, path)

		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create directory %s: %s", dir, err)
		}

		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create file %s: %s", path, err)
		}
	}
}
