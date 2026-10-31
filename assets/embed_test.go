package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtract(t *testing.T) {
	d := t.TempDir()
	if err := Extract("php", d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, ".keep")); err != nil {
		t.Fatal(err)
	}
}
