package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtract(t *testing.T) {
	d := t.TempDir()
	if err := Extract("webserver", d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"nginx.conf", "cron.conf"} {
		if _, err := os.Stat(filepath.Join(d, "supervisor", f)); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"entrypoint.sh", "install-php.sh"} {
		fi, err := os.Stat(filepath.Join(d, f))
		if err != nil || fi.Mode()&0o111 == 0 {
			t.Fatalf("%s not executable: %v %v", f, fi, err)
		}
	}
	if _, err := os.Stat(filepath.Join(d, "Dockerfile.legacy")); err != nil {
		t.Fatal(err)
	}
}

func TestHash(t *testing.T) {
	a, err := Hash("webserver")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Hash("webserver")
	m, _ := Hash("mkcert")
	if len(a) != 64 || a != b || a == m {
		t.Fatalf("php %q again %q mkcert %q", a, b, m)
	}
}
