package dotlodev_assets

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file added here but not to the go:embed line would never be scaffolded.
func TestEveryFileIsEmbedded(t *testing.T) {
	n := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, ".go") || d.Name() == ".DS_Store" {
			return err
		}
		n++
		disk, _ := os.ReadFile(path)
		if b, err := FS.ReadFile(filepath.ToSlash(path)); err != nil || string(b) != string(disk) {
			t.Errorf("%s is not embedded: %v", path, err)
		}
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("walked %d files: %v", n, err)
	}
}
