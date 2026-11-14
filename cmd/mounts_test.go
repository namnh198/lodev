package cmd

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// mountsFixture is project "shop" in a temp dir with Docker answering every call with no output.
func mountsFixture(t *testing.T) (*config.Global, *config.Project) {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	old := dockerOutput
	t.Cleanup(func() { dockerOutput = old })
	dockerOutput = func(...string) (string, error) { return "", nil }
	_, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	g, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	p, err := newProjectConfig(t.TempDir(), "shop", "php", "8.3", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return g, p
}

// The container mounts a copy of .lodev/homeadditional taken after the trust gate, not the repo folder:
// a container Docker restarts after a git pull copies only approved files into the home.
func TestPrepareMountsSnapshotsHomeAdditional(t *testing.T) {
	g, p := mountsFixture(t)
	target := filepath.Join(t.TempDir(), "dotfiles") // a symlinked folder is followed
	writeFile(t, filepath.Join(target, ".gitconfig"), "one")
	writeFile(t, filepath.Join(target, ".bash_aliases.example"), "# x")
	writeFile(t, filepath.Join(target, ".ssh", "config"), "Host *\n")
	os.Chmod(filepath.Join(target, ".ssh", "config"), 0o600)
	os.Symlink("../x", filepath.Join(target, "link"))
	repo := filepath.Join(p.Root, ".lodev", "homeadditional")
	os.MkdirAll(filepath.Dir(repo), 0o755)
	if err := os.Symlink(target, repo); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(config.Home(), "data", "web", "shop-web")
	snap := filepath.Join(home, "homeadditional")
	writeFile(t, filepath.Join(snap, "stale"), "x")
	cache := filepath.Join(home, ".npm", "x")
	writeFile(t, cache, "c")

	m, err := prepareMounts(g, p, "img")
	if err != nil {
		t.Fatal(err)
	}
	if m.HomeDir != home || m.ProjectHomeAdditional != snap || m.GlobalHomeAdditional != filepath.Join(config.Home(), "homeadditional") {
		t.Errorf("mounts = %+v", m)
	}
	if fi, err := os.Stat(m.GlobalHomeAdditional); err != nil || !fi.IsDir() {
		t.Errorf("global homeadditional: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(filepath.Join(snap, ".gitconfig")); string(b) != "one" {
		t.Errorf(".gitconfig = %q", b)
	}
	if fi, err := os.Stat(filepath.Join(snap, ".ssh", "config")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf(".ssh/config: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(snap, ".bash_aliases.example")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("example copied: %v", err)
	}
	if l, _ := os.Readlink(filepath.Join(snap, "link")); l != "../x" {
		t.Errorf("link = %q", l)
	}
	if _, err := os.Stat(filepath.Join(snap, "stale")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale file kept: %v", err)
	}
	if b, _ := os.ReadFile(cache); string(b) != "c" {
		t.Errorf("cache lost: %q", b)
	}

	os.Remove(repo)
	m, err = prepareMounts(g, p, "img")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snap); m.ProjectHomeAdditional != "" || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no folder: mount %q, snapshot %v", m.ProjectHomeAdditional, err)
	}
}

// Apache config runs commands (piped logs), so the container mounts a copy taken after the trust gate:
// a container Docker restarts after a git pull loads only approved config.
func TestPrepareMountsSnapshotsApacheConfig(t *testing.T) {
	g, p := mountsFixture(t)
	p.Webserver = "apache"
	writeFile(t, WebserverConfFile(p), "<VirtualHost>\n")
	writeFile(t, filepath.Join(p.Root, ".lodev", "apache", "logs.conf"), "CustomLog x\n")
	writeFile(t, filepath.Join(p.Root, ".lodev", "apache", "example.conf.example"), "# x\n")
	snap := filepath.Join(config.Home(), "data", "web", "shop-web", "apache")
	writeFile(t, filepath.Join(snap, "custom.d", "stale.conf"), "x")

	m, err := prepareMounts(g, p, "img")
	if err != nil {
		t.Fatal(err)
	}
	if m.ApacheSnapshot != snap {
		t.Errorf("ApacheSnapshot = %q", m.ApacheSnapshot)
	}
	if b, _ := os.ReadFile(filepath.Join(snap, "custom.d", "logs.conf")); string(b) != "CustomLog x\n" {
		t.Errorf("custom.d/logs.conf = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(snap, "webserver.conf")); string(b) != "<VirtualHost>\n" {
		t.Errorf("webserver.conf = %q", b)
	}
	if _, err := os.Stat(filepath.Join(snap, "custom.d", "stale.conf")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale file kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snap, "custom.d", "example.conf.example")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("example copied: %v", err)
	}

	p.Webserver = "nginx"
	m, err = prepareMounts(g, p, "img")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snap); m.ApacheSnapshot != "" || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("nginx: mount %q, snapshot %v", m.ApacheSnapshot, err)
	}
}

func TestRemoveMountsKeepsHome(t *testing.T) {
	_, p := mountsFixture(t)
	out, _, restore := ui.Capture(false)
	defer restore()
	home := filepath.Join(config.Home(), "data", "web", "shop-web")
	writeFile(t, filepath.Join(home, ".bash_history"), "ls\n")
	writeFile(t, filepath.Join(home, "homeadditional", ".gitconfig"), "x")
	writeFile(t, filepath.Join(home, "apache", "webserver.conf"), "x")

	if err := removeMounts(p, false, true); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.String(), "Kept "+home+" (caches and histories)")
	if _, err := os.Stat(filepath.Join(home, ".bash_history")); err != nil {
		t.Errorf("home removed: %v", err)
	}
	for _, snap := range []string{"homeadditional", "apache"} {
		if _, err := os.Stat(filepath.Join(home, snap)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s snapshot kept: %v", snap, err)
		}
	}
}

// The question at lodev create lists exactly the repo files the web mounts copy: a copied file
// nobody was asked about, or a listed one never copied, is a bug.
func TestTrustListsEveryCopiedFile(t *testing.T) {
	g, p := mountsFixture(t)
	p.Webserver = "apache"
	add := filepath.Join(p.Root, ".lodev", "homeadditional")
	writeFile(t, filepath.Join(add, ".gitconfig"), "x")
	writeFile(t, filepath.Join(add, ".config", "app.ini"), "x")
	writeFile(t, filepath.Join(add, ".bash_aliases.example"), "x")
	os.Symlink(".gitconfig", filepath.Join(add, "link"))
	writeFile(t, filepath.Join(p.Root, ".lodev", "apache", "logs.conf"), "x")
	writeFile(t, filepath.Join(p.Root, ".lodev", "apache", "example.conf.example"), "x")
	writeFile(t, WebserverConfFile(p), "<VirtualHost>\n") // the user's own: no lodev marker

	m, err := prepareMounts(g, p, "img")
	if err != nil {
		t.Fatal(err)
	}
	var copied []string
	for snap, repo := range map[string]string{m.ProjectHomeAdditional: ".lodev/homeadditional/", filepath.Join(m.ApacheSnapshot, "custom.d"): ".lodev/apache/"} {
		filepath.WalkDir(snap, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(snap, path)
			copied = append(copied, repo+filepath.ToSlash(rel))
			return nil
		})
	}
	if _, err := os.Stat(filepath.Join(m.ApacheSnapshot, "webserver.conf")); err == nil {
		copied = append(copied, ".lodev/apache-webserver.conf")
	}
	slices.Sort(copied)
	var listed []string
	for _, k := range trustKeys(t, p) {
		if strings.HasPrefix(k, ".lodev/homeadditional/") || strings.HasPrefix(k, ".lodev/apache") {
			listed = append(listed, k)
		}
	}
	if !slices.Equal(copied, listed) {
		t.Errorf("copied %q\nlisted %q", copied, listed)
	}
}
