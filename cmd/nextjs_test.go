package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lodev/internal/config"
)

// nextjsConfProject is a nextjs project in a temp dir, not registered.
func nextjsConfProject(t *testing.T) *config.Project {
	t.Helper()
	p, err := newProjectConfig(t.TempDir(), "web", "nextjs", "8.4", "nginx", "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWriteDaemonsNextjs(t *testing.T) {
	p := nextjsConfProject(t)
	if err := writeDaemons(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p.Root, ".lodev", "daemons", "nextjs.sh"))
	if err != nil || !strings.HasSuffix(string(b), "exec sh -c 'pnpm install && PORT=3000 exec pnpm run dev'\n") {
		t.Fatalf("nextjs.sh: %q %v", b, err)
	}
}

// A conf the user took over (marker line deleted) stays theirs on a nextjs project too.
func TestWriteWebserverConfKeepsTakenOverNextjs(t *testing.T) {
	p := nextjsConfProject(t)
	f := filepath.Join(p.Root, ".lodev", "nginx-webserver.conf")
	writeFile(t, f, "server { listen 80; }\n")
	if err := writeWebserverConf(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "server { listen 80; }\n" {
		t.Fatalf("taken-over conf rewritten: %q", b)
	}
}
