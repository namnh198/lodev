package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/hosts"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

// volumeFixture is a magento project "shop" in bind mode with volume_dirs pub/media; docker calls are
// recorded and answered by reply (nil: success, empty output).
func volumeFixture(t *testing.T, reply func(args string) (string, error)) (p *config.Project, g *config.Global, calls *[]string) {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	g, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	p = &config.Project{Name: "shop", Type: "magento", Root: t.TempDir(), PerformanceMode: "bind", VolumeDirs: []string{"pub/media"}}
	calls = &[]string{}
	old := dockerOutput
	t.Cleanup(func() { dockerOutput = old })
	dockerOutput = func(a ...string) (string, error) {
		s := strings.Join(a, " ")
		*calls = append(*calls, s)
		if reply != nil {
			return reply(s)
		}
		return "", nil
	}
	return p, g, calls
}

// missing makes every `volume inspect` fail, as for volumes that do not exist yet.
func missing(s string) (string, error) {
	if strings.HasPrefix(s, "volume inspect") {
		return "", errors.New("No such volume")
	}
	return "", nil
}

func TestEnsureVolumesCreatesAndSeeds(t *testing.T) {
	p, g, calls := volumeFixture(t, missing)
	os.MkdirAll(filepath.Join(p.Root, "pub", "media"), 0o755)
	out, _, restore := ui.Capture(false)
	defer restore()

	vols, err := ensureVolumes(p, g, "lodev/shop-web")
	if err != nil {
		t.Fatal(err)
	}
	want := []templates.Volume{
		{Name: "lodev-shop-var", Dir: "var"}, {Name: "lodev-shop-generated", Dir: "generated"},
		{Name: "lodev-shop-pub-media", Dir: "pub/media"},
	}
	if len(vols) != len(want) {
		t.Fatalf("volumes = %v", vols)
	}
	for i := range want {
		if vols[i] != want[i] {
			t.Errorf("volume %d = %v, want %v", i, vols[i], want[i])
		}
	}
	got := strings.Join(*calls, "\n")
	mustContain(t, got,
		"volume create --label lodev.project=shop --label lodev.dir=var lodev-shop-var",
		"volume create --label lodev.project=shop --label lodev.dir=pub/media lodev-shop-pub-media",
		"run --rm --user 0 --entrypoint cp -v "+filepath.Join(p.Root, "pub/media")+":/from:ro -v lodev-shop-pub-media:/to lodev/shop-web -a /from/. /to/")
	if n := strings.Count(got, "--entrypoint cp"); n != 1 {
		t.Errorf("%d copies, want 1 (only pub/media is seeded):\n%s", n, got)
	}
	mustContain(t, out.String(), "pub/media: copied from the host",
		"var: starts empty (host files are untouched, the container no longer sees them)",
		"run lodev npm install", "lodev magento setup:di:compile if you need generated code up front",
		"lodev config --performance-mode=none turns it off")
}

func TestEnsureVolumesHintDaemons(t *testing.T) {
	p, g, _ := volumeFixture(t, missing)
	out, _, restore := ui.Capture(false)
	defer restore()
	if _, err := ensureVolumes(p, g, "img"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "daemons retry") {
		t.Errorf("daemon hint without daemons: %s", out)
	}
	p.WebExtraDaemons = []config.Daemon{{Name: "vite", Command: "npm run dev"}}
	p.Name = "shop2" // its volumes are new again
	if _, err := ensureVolumes(p, g, "img"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.String(), "lodev config --performance-mode=none turns it off; daemons retry on their own, lodev restart forces it")
}

func TestEnsureVolumesSeedFolderMissingOnHost(t *testing.T) {
	p, g, calls := volumeFixture(t, missing)
	if _, err := ensureVolumes(p, g, "img"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*calls, "\n"); strings.Contains(got, "--entrypoint cp") || !strings.Contains(got, "lodev-shop-pub-media") {
		t.Errorf("want create without copy:\n%s", got)
	}
}

func TestEnsureVolumesExistingOnlyInspected(t *testing.T) {
	p, g, calls := volumeFixture(t, nil)
	out, _, restore := ui.Capture(false)
	defer restore()
	vols, err := ensureVolumes(p, g, "img")
	if err != nil || len(vols) != 3 {
		t.Fatalf("%v %v", vols, err)
	}
	for _, c := range *calls {
		if !strings.HasPrefix(c, "volume inspect") && !strings.HasPrefix(c, "volume ls") {
			t.Errorf("unexpected docker call %q", c)
		}
	}
	if out.Len() != 0 {
		t.Errorf("nothing was created, yet: %s", out)
	}
}

func TestEnsureVolumesSeedFailureRemovesVolume(t *testing.T) {
	p, g, calls := volumeFixture(t, func(s string) (string, error) {
		if strings.HasPrefix(s, "run ") {
			return "", errors.New("cp failed")
		}
		return missing(s)
	})
	os.MkdirAll(filepath.Join(p.Root, "pub", "media"), 0o755)
	if _, err := ensureVolumes(p, g, "img"); err == nil || !strings.Contains(err.Error(), "cp failed") {
		t.Fatalf("err = %v", err)
	}
	if got := *calls; got[len(got)-1] != "volume rm lodev-shop-pub-media" {
		t.Errorf("calls = %v", got)
	}
}

func TestEnsureVolumesSkipsSymlink(t *testing.T) {
	p, g, calls := volumeFixture(t, missing)
	os.Symlink(t.TempDir(), filepath.Join(p.Root, "var"))
	_, errOut, restore := ui.Capture(false)
	defer restore()
	vols, err := ensureVolumes(p, g, "img")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vols {
		if v.Dir == "var" {
			t.Error("symlinked var got a volume")
		}
	}
	if strings.Contains(strings.Join(*calls, "\n"), "lodev-shop-var") {
		t.Errorf("calls touched the var volume: %v", *calls)
	}
	mustContain(t, errOut.String(), "var is a symlink on the host; not putting it in a volume")
}

func TestEnsureVolumesListsUnused(t *testing.T) {
	p, g, _ := volumeFixture(t, func(s string) (string, error) {
		if strings.HasPrefix(s, "volume ls") {
			return "lodev-shop-var\nlodev-shop-old", nil
		}
		return "", nil
	})
	out, _, restore := ui.Capture(false)
	defer restore()
	if _, err := ensureVolumes(p, g, "img"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.String(), "unused volumes: lodev-shop-old (lodev remove --volumes, or docker volume rm …)")
}

func TestEnsureVolumesNoneListsUnused(t *testing.T) {
	p, g, _ := volumeFixture(t, func(s string) (string, error) {
		if strings.HasPrefix(s, "volume ls") {
			return "lodev-shop-var", nil
		}
		return "", nil
	})
	p.PerformanceMode = "none"
	out, _, restore := ui.Capture(false)
	defer restore()
	vols, err := ensureVolumes(p, g, "img")
	if err != nil || len(vols) != 0 {
		t.Fatalf("%v %v", vols, err)
	}
	mustContain(t, out.String(), "unused volumes: lodev-shop-var")
}

func deleteFixture(t *testing.T, reply func(string) (string, error)) *[]string {
	t.Helper()
	hf := filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(hf, []byte("127.0.0.1 localhost\n"), 0o644)
	t.Cleanup(func(o string) func() { return func() { hosts.File = o } }(hosts.File))
	hosts.File = hf
	_, _, calls := volumeFixture(t, reply)
	return calls
}

func labelled(s string) (string, error) {
	if strings.HasPrefix(s, "volume ls") {
		return "lodev-shop-var\nlodev-shop-pub-media", nil
	}
	return "", nil
}

func TestDeleteKeepsVolumes(t *testing.T) {
	calls := deleteFixture(t, labelled)
	p, _ := newProjectConfig(t.TempDir(), "shop", "php", "8.3", "", "")
	out, _, restore := ui.Capture(false)
	defer restore()
	if err := deleteProject(p, false, false, true); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.String(), "kept volumes: lodev-shop-var, lodev-shop-pub-media (lodev remove --volumes deletes them)")
	if strings.Contains(strings.Join(*calls, "\n"), "volume rm") {
		t.Errorf("volumes removed without --volumes: %v", *calls)
	}
}

func TestDeleteDropsVolumesAfterConfirm(t *testing.T) {
	calls := deleteFixture(t, labelled)
	p, _ := newProjectConfig(t.TempDir(), "shop", "php", "8.3", "", "")
	if err := deleteProject(p, false, true, true); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(*calls, "\n")
	mustContain(t, got, "volume ls -q -f label=lodev.project=shop", "volume rm lodev-shop-var", "volume rm lodev-shop-pub-media")
}
