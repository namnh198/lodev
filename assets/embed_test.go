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
	if _, err := os.Stat(filepath.Join(d, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "supervisor", "nginx.conf")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(d, "entrypoint.sh"))
	if err != nil || fi.Mode()&0o111 == 0 {
		t.Fatalf("entrypoint.sh not executable: %v %v", fi, err)
	}
}
