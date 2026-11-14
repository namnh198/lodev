package assets

import (
	"os"
	"path/filepath"
	"strings"
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
	for _, f := range []string{"entrypoint.sh", "install-php.sh", "healthcheck.sh"} {
		fi, err := os.Stat(filepath.Join(d, f))
		if err != nil || fi.Mode()&0o111 == 0 {
			t.Fatalf("%s not executable: %v %v", f, fi, err)
		}
	}
	if _, err := os.Stat(filepath.Join(d, "Dockerfile.legacy")); err != nil {
		t.Fatal(err)
	}
	// The healthcheck runs in the project folder; a project's own supervisord.conf there must not be read.
	hc, _ := os.ReadFile(filepath.Join(d, "healthcheck.sh"))
	if !strings.Contains(string(hc), "supervisorctl -c /etc/supervisor/supervisord.conf ") {
		t.Error("healthcheck.sh: supervisorctl must name its config")
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

// TestSkel: the lodev /etc/skel carries the history size, git defaults and the ll alias, nothing
// from DDEV, and no .ssh (the image has no ssh client).
func TestSkel(t *testing.T) {
	for file, c := range map[string]struct{ want, not []string }{
		"webserver/skel/.bashrc":       {want: []string{"HISTSIZE=10000", "HISTFILESIZE=10000", "shopt -s histappend", "HISTCONTROL=ignoreboth", `"$HOME/bin"`, `"$HOME/.local/bin"`}},
		"webserver/skel/.gitconfig":    {want: []string{"autocrlf = input", "directory = *"}, not: []string{"[user]"}},
		"webserver/skel/.bash_aliases": {want: []string{"alias ll='ls -la'"}},
	} {
		b, err := FS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range c.want {
			if !strings.Contains(string(b), w) {
				t.Errorf("%s lacks %q", file, w)
			}
		}
		for _, n := range c.not {
			if strings.Contains(string(b), n) {
				t.Errorf("%s has %q", file, n)
			}
		}
	}
	if _, err := FS.ReadFile("webserver/skel/.ssh/config"); err == nil {
		t.Error("skel has .ssh/config, but the image has no ssh client")
	}
}

// TestHashCoversSkel: Hash reads every file under webserver, so the image tag changes with a skel
// change as long as the skel files (dotfiles too) are inside the embedded tree.
func TestHashCoversSkel(t *testing.T) {
	for _, f := range []string{".bashrc", ".profile", ".gitconfig", ".bash_aliases"} {
		if _, err := FS.ReadFile("webserver/skel/" + f); err != nil {
			t.Fatalf("skel/%s is not embedded: %v", f, err)
		}
	}
	whole, err := Hash("webserver")
	if err != nil {
		t.Fatal(err)
	}
	skel, err := Hash("webserver/skel")
	if err != nil || skel == whole || skel == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("skel hash %q (err %v) is empty or equals the whole tree's %q", skel, err, whole)
	}
}
