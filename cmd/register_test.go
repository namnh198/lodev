package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

// runRoot runs a root command and resets its flags afterwards.
func runRoot(t *testing.T, args ...string) error {
	t.Helper()
	c, _, _ := rootCmd.Find(args[:1])
	t.Cleanup(func() {
		c.Flags().VisitAll(func(f *pflag.Flag) { f.Value.Set(f.DefValue); f.Changed = false })
		rootCmd.SetArgs(nil)
	})
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// stubStart records which project start/restart would run, without Docker.
func stubStart(t *testing.T) *[]string {
	t.Helper()
	oldStart, oldRestart := startFn, restartFn
	t.Cleanup(func() { startFn, restartFn = oldStart, oldRestart })
	var got []string
	startFn = func(p *config.Project, _ startOpts) error { got = append(got, "start "+p.Name); return nil }
	restartFn = func(p *config.Project) error { got = append(got, "restart "+p.Name); return nil }
	return &got
}

func register(t *testing.T, name, path string) {
	t.Helper()
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg[name] = registry.Entry{Path: path}
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestStartNeedsAProject(t *testing.T) {
	for _, cmd := range []string{"start", "restart"} {
		t.Run(cmd+" unregistered", func(t *testing.T) {
			newShop(t)
			got := stubStart(t)
			err := runRoot(t, cmd)
			if err == nil || !strings.Contains(err.Error(), "project does not exist") || !strings.Contains(err.Error(), "run lodev create") {
				t.Errorf("error %v", err)
			}
			if len(*got) != 0 {
				t.Errorf("ran %v", *got)
			}
		})
		t.Run(cmd+" no config", func(t *testing.T) {
			t.Setenv("LODEV_HOME", t.TempDir())
			t.Chdir(t.TempDir())
			stubStart(t)
			err := runRoot(t, cmd)
			if err == nil || !strings.Contains(err.Error(), "project does not exist") || !strings.Contains(err.Error(), "run lodev create") {
				t.Errorf("error %v", err)
			}
		})
		t.Run(cmd+" registered under an old name", func(t *testing.T) {
			root := newShop(t)
			register(t, "old-shop", root) // renamed in config.yaml: start retires the old name
			got := stubStart(t)
			if err := runRoot(t, cmd); err != nil || len(*got) != 1 || (*got)[0] != cmd+" shop" {
				t.Errorf("err %v, ran %v", err, *got)
			}
		})
	}
}

func registeredPath(t *testing.T, name string) string {
	t.Helper()
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	return reg[name].Path
}

func TestCreateRegisters(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "blog")
	os.Mkdir(root, 0o755)
	t.Chdir(root)
	if err := runRoot(t, "create", "--no-ask"); err != nil {
		t.Fatal(err)
	}
	if got := registeredPath(t, "blog"); got != root {
		t.Errorf("registered %q, want %q", got, root)
	}
}

func TestCreateRegistersExistingConfig(t *testing.T) {
	root := newShop(t)
	before, _ := os.ReadFile(config.ProjectPath(root))
	_, e, restore := ui.Capture(false)
	t.Cleanup(restore)
	if err := runRoot(t, "create", "--no-ask"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.String(), ".lodev/config.yaml already exists") {
		t.Errorf("no warning: %q", e.String())
	}
	if after, _ := os.ReadFile(config.ProjectPath(root)); string(after) != string(before) {
		t.Errorf("config changed:\n%s", after)
	}
	if got := registeredPath(t, "shop"); got != root {
		t.Errorf("registered %q", got)
	}
	if err := runRoot(t, "create", "--no-ask"); err == nil || !strings.Contains(err.Error(), "already initialized") {
		t.Errorf("second create: %v", err)
	}
}
