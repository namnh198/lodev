package cmd

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// xdebugProject is a project whose .lodev/config.yaml holds a comment and enable_xdebug as given.
func xdebugProject(t *testing.T, enabled bool) *config.Project {
	t.Helper()
	p := &config.Project{Name: "shop", Type: "php", PHP: "8.4", Webserver: "nginx", Root: t.TempDir(), EnableXdebug: enabled}
	body := "# mine\nname: shop\n"
	if enabled {
		body += "enable_xdebug: true\n"
	}
	if err := os.MkdirAll(p.Root+"/.lodev", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.ProjectPath(p.Root), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// stubXdebugExec records the root scripts run in the web container.
func stubXdebugExec(t *testing.T, running bool, fail error) *[]string {
	t.Helper()
	oldRun, oldUp := dockerRun, containerRunning
	t.Cleanup(func() { dockerRun, containerRunning = oldRun, oldUp })
	containerRunning = func(string) bool { return running }
	var scripts []string
	dockerRun = func(a ...string) error {
		if i := slices.Index(a, "-c"); i > 0 && a[i-1] == "sh" {
			scripts = append(scripts, a[i+1])
			return fail
		}
		return nil
	}
	return &scripts
}

func configText(t *testing.T, p *config.Project) string {
	t.Helper()
	b, err := os.ReadFile(config.ProjectPath(p.Root))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func localText(t *testing.T, p *config.Project) string {
	t.Helper()
	b, err := os.ReadFile(localConfigPath(p.Root))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSetXdebugRunning(t *testing.T) {
	p := xdebugProject(t, false)
	scripts := stubXdebugExec(t, true, nil)
	before := configText(t, p)
	if err := setXdebug(p, true); err != nil {
		t.Fatal(err)
	}
	if len(*scripts) != 1 || (*scripts)[0] != xdebugScript("on") {
		t.Errorf("scripts %q", *scripts)
	}
	if got := localText(t, p); !strings.Contains(got, "enable_xdebug: true") {
		t.Errorf("local config %q", got)
	}
	if err := setXdebug(p, false); err != nil {
		t.Fatal(err)
	}
	if got := localText(t, p); strings.Contains(got, "enable_xdebug") {
		t.Errorf("local config after off %q", got)
	}
	if got := configText(t, p); got != before {
		t.Errorf("config.yaml changed: %q", got)
	}
}

// TestSetXdebugOffOverridesCommitted: config.yaml keeps enable_xdebug: true, so off must say false locally.
func TestSetXdebugOffOverridesCommitted(t *testing.T) {
	p := xdebugProject(t, true)
	stubXdebugExec(t, false, nil)
	_, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	before := configText(t, p)
	if err := setXdebug(p, false); err != nil {
		t.Fatal(err)
	}
	if got := localText(t, p); !strings.Contains(got, "enable_xdebug: false") {
		t.Errorf("local config %q", got)
	}
	if got := configText(t, p); got != before {
		t.Errorf("config.yaml changed: %q", got)
	}
}

// TestSetXdebugOffWithoutLocalFile: nothing to remove, nothing created.
func TestSetXdebugOffWithoutLocalFile(t *testing.T) {
	p := xdebugProject(t, false)
	stubXdebugExec(t, false, nil)
	_, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	if err := setXdebug(p, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(localConfigPath(p.Root)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.local.yaml created: %v", err)
	}
}

func TestSetXdebugStoppedOnlySaves(t *testing.T) {
	p := xdebugProject(t, false)
	scripts := stubXdebugExec(t, false, nil)
	o, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	if err := setXdebug(p, true); err != nil {
		t.Fatal(err)
	}
	if len(*scripts) != 0 || !strings.Contains(localText(t, p), "enable_xdebug: true") {
		t.Errorf("scripts %q, local config %q", *scripts, localText(t, p))
	}
	if !strings.Contains(o.String(), "xdebug on: applies on the next lodev start") {
		t.Errorf("output %q", o.String())
	}
}

func TestSetXdebugFailureKeepsConfig(t *testing.T) {
	p := xdebugProject(t, false)
	stubXdebugExec(t, true, errors.New("boom"))
	if err := setXdebug(p, true); err == nil {
		t.Fatal("no error")
	}
	if _, err := os.Stat(localConfigPath(p.Root)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.local.yaml written: %v", err)
	}
}

func TestToggleXdebugFollowsConfig(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		if got := xdebugTarget("toggle", &config.Project{EnableXdebug: enabled}); got != !enabled {
			t.Errorf("toggle from %v: %v", enabled, got)
		}
	}
	if !xdebugTarget("on", &config.Project{EnableXdebug: true}) || xdebugTarget("off", &config.Project{}) {
		t.Error("on/off")
	}
}

// TestSyncXdebug: start turns Xdebug on when enabled, and off only if a kept container still has it.
func TestSyncXdebug(t *testing.T) {
	scripts := stubXdebugExec(t, true, nil)
	syncXdebug(&config.Project{Name: "shop", Root: t.TempDir(), EnableXdebug: true})
	syncXdebug(&config.Project{Name: "shop", Root: t.TempDir()})
	if len(*scripts) != 2 || (*scripts)[0] != xdebugScript("on") ||
		!strings.HasPrefix((*scripts)[1], "[ ! -e ") || !strings.Contains((*scripts)[1], xdebugScript("off")) {
		t.Errorf("scripts %q", *scripts)
	}
}
