package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

// A new file takes mode, a symlink is written through, and no temp file is left.
func TestWrite(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.yaml")
	if err := Write(filepath.Join(dir, "sub", "new.yaml"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "sub", "new.yaml")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("new file: %v %v", fi, err)
	}
	os.WriteFile(real, []byte("old"), 0o644)
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(real); string(b) != "new" {
		t.Errorf("target = %q", b)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced by a file")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) > 0 {
		t.Errorf("temp files left: %v", left)
	}
}
