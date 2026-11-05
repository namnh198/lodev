package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/ui"
)

type dataFix struct {
	calls     []string
	volumes   map[string]bool // named volumes that exist
	runErr    error
	container bool // lodev-<svc> containers exist
}

// stubServiceData swaps the docker seams prepareServiceData uses.
func stubServiceData(t *testing.T) *dataFix {
	t.Helper()
	f := &dataFix{volumes: map[string]bool{}, container: true}
	oldRun, oldOut := dockerRun, dockerOutput
	t.Cleanup(func() { dockerRun, dockerOutput = oldRun, oldOut })
	dockerRun = func(a ...string) error { f.calls = append(f.calls, strings.Join(a, " ")); return f.runErr }
	dockerOutput = func(a ...string) (string, error) {
		f.calls = append(f.calls, strings.Join(a, " "))
		switch {
		case a[0] == "volume" && a[1] == "inspect":
			if !f.volumes[a[2]] {
				return "", errors.New("no such volume")
			}
		case a[0] == "ps" && f.container:
			return "abc123", nil
		}
		return "", nil
	}
	return f
}

func mountGlobal(t *testing.T) *config.Global {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	g, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	g.MountVolume = true
	return g
}

func runs(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, "run ") || strings.HasPrefix(c, "stop ") {
			out = append(out, c)
		}
	}
	return out
}

func TestPrepareServiceDataCopies(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-mysql"] = true
	g := mountGlobal(t)
	out, _, restore := ui.Capture(false)
	defer restore()
	if err := prepareServiceData(g, []string{"mysql", "mailpit"}); err != nil {
		t.Fatal(err)
	}
	path := g.MountPath("mysql")
	want := []string{"stop lodev-mysql", "run --rm --user 0 --entrypoint cp -v lodev-mysql:/from -v " + path + ":/to mysql:8.0 -a /from/. /to/"}
	if got := runs(f.calls); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	mustContain(t, out.String(), "Copied volume lodev-mysql to "+path+" (the volume is kept)")
}

func TestPrepareServiceDataKeepsData(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-mysql"] = true
	g := mountGlobal(t)
	writeFile(t, filepath.Join(g.MountPath("mysql"), "ibdata1"), "x")
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("touched a folder with data: %v", got)
	}
}

func TestPrepareServiceDataNoVolume(t *testing.T) {
	f := stubServiceData(t)
	g := mountGlobal(t)
	if err := prepareServiceData(g, []string{"redis"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("copied without a volume: %v", got)
	}
	if fi, err := os.Stat(g.MountPath("redis")); err != nil || !fi.IsDir() {
		t.Errorf("folder not created: %v", err)
	}
}

func TestPrepareServiceDataNoContainer(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-redis"], f.container = true, false
	g := mountGlobal(t)
	if err := prepareServiceData(g, []string{"redis"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) != 1 || !strings.HasPrefix(got[0], "run ") {
		t.Errorf("calls %v", got)
	}
}

func TestPrepareServiceDataOpenSearchMode(t *testing.T) {
	stubServiceData(t)
	g := mountGlobal(t)
	if err := prepareServiceData(g, []string{"opensearch"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(g.MountPath("opensearch")); err != nil || fi.Mode().Perm() != 0o777 {
		t.Errorf("opensearch folder %v %v", fi.Mode(), err)
	}
}

func TestPrepareServiceDataOff(t *testing.T) {
	f := stubServiceData(t)
	g := mountGlobal(t)
	g.MountVolume = false
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) > 0 {
		t.Errorf("docker calls with mount_volume off: %v", f.calls)
	}
	if _, err := os.Stat(filepath.Join(g.DataPath(), "mysql")); err == nil {
		t.Error("folder created with mount_volume off")
	}
}

func TestPrepareServiceDataCopyFails(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-mysql"], f.runErr = true, errors.New("boom")
	g := mountGlobal(t)
	err := prepareServiceData(g, []string{"mysql"})
	if err == nil || !strings.Contains(err.Error(), g.MountPath("mysql")) || !strings.Contains(err.Error(), "empty") {
		t.Errorf("err = %v", err)
	}
}
