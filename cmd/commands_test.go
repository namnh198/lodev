package cmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/commands"
	"lodev/internal/config"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

// customProject makes the project "shop" of type typ (cwd is its root), registered or not.
func customProject(t *testing.T, typ string, registered bool) string {
	t.Helper()
	root := newShop(t)
	writeShop(t, root, "name: shop\ntype: "+typ+"\nphp: \"8.3\"\nservices: [mysql]\n")
	if registered {
		register(t, "shop", root)
	}
	return root
}

func globalCommand(t *testing.T, kind, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(config.Home(), "commands", kind, name), body)
}

// registerFor runs registerCustom for wd, removes what it added at cleanup, and returns the
// added names and the warnings it printed.
func registerFor(t *testing.T, wd string) (names []string, stderr string) {
	t.Helper()
	_, e, restore := ui.Capture(false)
	added := registerCustom(rootCmd, wd)
	restore()
	t.Cleanup(func() { rootCmd.RemoveCommand(added...) })
	for _, c := range added {
		names = append(names, c.Name())
	}
	return names, e.String()
}

// withoutShipped leaves lodev's embedded commands out, so a test sees only the files it writes.
func withoutShipped(t *testing.T) {
	t.Helper()
	old := shippedCommands
	t.Cleanup(func() { shippedCommands = old })
	shippedCommands = func() []commands.Command { return nil }
}

func stubHostRun(t *testing.T) **exec.Cmd {
	t.Helper()
	old, oldUp := hostRun, containerRunning
	t.Cleanup(func() { hostRun, containerRunning = old, oldUp })
	containerRunning = func(string) bool { return false }
	var got *exec.Cmd
	hostRun = func(c *exec.Cmd) error { got = c; return nil }
	return &got
}

func TestCustomWebCommand(t *testing.T) {
	root := customProject(t, "php", true)
	writeFile(t, filepath.Join(root, ".lodev/commands/web/hi"), "echo \"$@\"\n")
	oldRun, oldReady, oldUp := dockerRun, userReady, containerRunning
	t.Cleanup(func() { dockerRun, userReady, containerRunning = oldRun, oldReady, oldUp })
	userReady = func(*config.Project, string) bool { return true }
	containerRunning = func(string) bool { return true }
	var got []string
	dockerRun = func(a ...string) error { got = a; return nil }
	registerFor(t, root)
	if err := runRoot(t, "hi", "a b", ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"bash", "-c", "echo \"$@\"\n", "hi", "a b", ""}
	if len(got) < len(want) || !slices.Equal(got[len(got)-len(want):], want) {
		t.Fatalf("docker %q", got)
	}
}

func TestCustomHostCommand(t *testing.T) {
	root := customProject(t, "php", true)
	script := "echo \"$LODEV_URL\"\n"
	writeFile(t, filepath.Join(root, ".lodev/commands/host/hi"), script)
	got := stubHostRun(t)
	wd := filepath.Join(root, "app", "Http")
	os.MkdirAll(wd, 0o755)
	t.Chdir(wd)
	registerFor(t, wd)
	if err := runRoot(t, "hi", "x"); err != nil {
		t.Fatal(err)
	}
	c := *got
	if c == nil {
		t.Fatal("not run")
	}
	if !slices.Equal(c.Args[:3], []string{"bash", "-c", script}) || c.Args[3] != "hi" || c.Args[4] != "x" {
		t.Errorf("args %q", c.Args)
	}
	if c.Dir != root {
		t.Errorf("dir %q, want %q", c.Dir, root)
	}
	for _, e := range []string{"LODEV_PROJECT=shop", "LODEV_ROOT=" + root, "LODEV_PWD=" + wd, "LODEV_PROJECT_STATUS=stopped"} {
		if !slices.Contains(c.Env, e) {
			t.Errorf("env lacks %s", e)
		}
	}
}

func TestHostCommandDropsInheritedLodevVars(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	stale := []string{"LODEV_BIN", "LODEV_PWD", "LODEV_PROJECT", "LODEV_ROOT", "LODEV_TYPE", "LODEV_URL", "LODEV_WEB_CONTAINER",
		"LODEV_CONTAINER_USER", "LODEV_WORKDIR", "LODEV_PROJECT_STATUS", "LODEV_DB_TYPE", "LODEV_DB_HOST", "LODEV_DB_PORT",
		"LODEV_DB_USER", "LODEV_DB_PASSWORD", "LODEV_DB_NAME"}
	for _, k := range stale {
		t.Setenv(k, "stale")
	}
	t.Setenv("LODEV_DEBUG", "1")
	globalCommand(t, "host", "hi", "echo hi\n")
	got := stubHostRun(t)
	t.Chdir(t.TempDir())
	wd, _ := os.Getwd()
	registerFor(t, wd)
	if err := runRoot(t, "hi"); err != nil {
		t.Fatal(err)
	}
	env := (*got).Env
	for _, e := range env {
		if k, _, _ := strings.Cut(e, "="); slices.Contains(stale, k) && k != "LODEV_BIN" && k != "LODEV_PWD" {
			t.Errorf("stale %s inherited", e)
		}
	}
	for _, k := range []string{"LODEV_HOME=", "LODEV_DEBUG=1", "LODEV_PWD=" + wd} {
		if !slices.ContainsFunc(env, func(e string) bool { return strings.HasPrefix(e, k) }) {
			t.Errorf("env lacks %s", k)
		}
	}
	if n := slices.IndexFunc(env, func(e string) bool { return strings.HasPrefix(e, "LODEV_BIN=") }); n < 0 || env[n] == "LODEV_BIN=stale" {
		t.Errorf("LODEV_BIN not fresh")
	}
}

func TestCustomPrecedence(t *testing.T) {
	withoutShipped(t)
	root := customProject(t, "php", true)
	globalCommand(t, "host", "x", "# Description: global x\n")
	writeFile(t, filepath.Join(root, ".lodev/commands/web/x"), "# Description: project x\n")
	globalCommand(t, "web", "y", "# Description: web y\n")
	globalCommand(t, "host", "y", "# Description: host y\n")
	globalCommand(t, "web", "composer", templates.GeneratedMarker+"\nexec composer \"$@\"\n")
	names, stderr := registerFor(t, root)
	if !slices.Equal(names, []string{"x", "y"}) {
		t.Fatalf("registered %q", names)
	}
	x, _, _ := rootCmd.Find([]string{"x"})
	y, _, _ := rootCmd.Find([]string{"y"})
	if x.Short != "project x" || y.Short != "web y" {
		t.Errorf("x %q, y %q", x.Short, y.Short)
	}
	if n := strings.Count(stderr, "\n"); n != 1 || !strings.Contains(stderr, `"y" is also in`) || !strings.Contains(stderr, "using the web one") {
		t.Errorf("stderr %q", stderr)
	}
}

func TestCustomTypeFilter(t *testing.T) {
	withoutShipped(t)
	root := customProject(t, "laravel", true)
	globalCommand(t, "web", "m", "# Type: magento\n")
	if names, _ := registerFor(t, root); len(names) != 0 {
		t.Errorf("laravel project registered %q", names)
	}
	t.Chdir(t.TempDir())
	wd, _ := os.Getwd()
	if names, _ := registerFor(t, wd); !slices.Equal(names, []string{"m"}) {
		t.Errorf("outside a project registered %q", names)
	}
}

func TestCustomUnregisteredProject(t *testing.T) {
	withoutShipped(t)
	root := customProject(t, "php", false)
	writeFile(t, filepath.Join(root, ".lodev/commands/host/evil"), "rm -rf ~\n")
	names, stderr := registerFor(t, root)
	if len(names) != 0 {
		t.Errorf("registered %q", names)
	}
	hint := ".lodev/commands is not loaded: run lodev create to review the project's commands"
	if strings.Count(stderr, hint) != 1 {
		t.Errorf("stderr %q", stderr)
	}
}

func TestCustomBuiltinNames(t *testing.T) {
	withoutShipped(t)
	root := customProject(t, "php", true)
	builtins := []string{"start", "up", "ps", "st", "desc", "init", "help"}
	for _, n := range builtins {
		globalCommand(t, "host", n, "echo hijacked\n")
	}
	names, stderr := registerFor(t, root)
	if len(names) != 0 {
		t.Errorf("registered %q", names)
	}
	for _, n := range builtins {
		if !strings.Contains(stderr, `"`+n+`" is a lodev command; skipped`) {
			t.Errorf("no warning for %s: %q", n, stderr)
		}
	}
	if c, _, _ := rootCmd.Find([]string{"up"}); c.Name() != "start" {
		t.Errorf("up is %s", c.Name())
	}
}

func TestCustomBrokenConfig(t *testing.T) {
	withoutShipped(t)
	root := customProject(t, "php", true)
	writeShop(t, root, "name: [\n")
	globalCommand(t, "host", "g", "echo g\n")
	writeFile(t, filepath.Join(root, ".lodev/commands/web/pw"), "echo pw\n")
	if names, _ := registerFor(t, root); !slices.Equal(names, []string{"g"}) {
		t.Errorf("registered %q", names)
	}
	rootCmd.SetOut(io.Discard)
	t.Cleanup(func() { rootCmd.SetOut(nil) })
	if err := runRoot(t, "version"); err != nil {
		t.Errorf("version: %v", err)
	}
}

func TestCustomWebOutsideProject(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	globalCommand(t, "web", "hi", "echo hi\n")
	t.Chdir(t.TempDir())
	wd, _ := os.Getwd()
	registerFor(t, wd)
	if err := runRoot(t, "hi"); err == nil || !strings.Contains(err.Error(), "not a lodev project") {
		t.Errorf("error %v", err)
	}
}

// stubWebRun makes the web container look running and records the docker args of each run.
func stubWebRun(t *testing.T) *[]string {
	t.Helper()
	oldRun, oldReady, oldUp := dockerRun, userReady, containerRunning
	t.Cleanup(func() { dockerRun, userReady, containerRunning = oldRun, oldReady, oldUp })
	userReady = func(*config.Project, string) bool { return true }
	containerRunning = func(string) bool { return true }
	var got []string
	dockerRun = func(a ...string) error { got = a; return nil }
	return &got
}

func shippedScript(t *testing.T, name string) string {
	t.Helper()
	for _, c := range shippedCommands() {
		if c.Kind == commands.Web && c.Name == name {
			return c.Script
		}
	}
	t.Fatalf("%s is not shipped", name)
	return ""
}

func TestShortcutRunsShippedScript(t *testing.T) {
	root := customProject(t, "magento", true)
	got := stubWebRun(t)
	names, _ := registerFor(t, root)
	if slices.Contains(names, "artisan") || !slices.Contains(names, "magento") {
		t.Errorf("magento project registered %q", names)
	}
	if err := runRoot(t, "magento", "cache:flush"); err != nil {
		t.Fatal(err)
	}
	want := []string{"bash", "-c", shippedScript(t, "magento"), "magento", "cache:flush"}
	if len(*got) < len(want) || !slices.Equal((*got)[len(*got)-len(want):], want) {
		t.Errorf("docker %q", *got)
	}
}

// A separate test: the magento one leaves `magento` on rootCmd until it ends, which would make
// builtinNames skip it here whatever the type filter does.
func TestShortcutTypeFilterLaravel(t *testing.T) {
	root := customProject(t, "laravel", true)
	names, _ := registerFor(t, root)
	if slices.Contains(names, "magento") || !slices.Contains(names, "artisan") {
		t.Errorf("laravel project registered %q", names)
	}
}

func TestSyncOnRun(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	rootCmd.SetOut(io.Discard)
	t.Cleanup(func() { rootCmd.SetOut(nil) })
	if err := runRoot(t, "version"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.Home(), "commands")); err == nil {
		t.Error("version synced the shipped commands")
	}
	if err := runRoot(t, "list"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(config.Home(), "commands", "web", "composer")); err != nil {
		t.Errorf("list did not sync: %v", err)
	}
}

func TestShippedUserOverride(t *testing.T) {
	root := customProject(t, "php", true)
	globalCommand(t, "web", "composer", "echo mine\n")
	got := stubWebRun(t)
	registerFor(t, root)
	if err := runRoot(t, "composer", "install"); err != nil {
		t.Fatal(err)
	}
	want := []string{"bash", "-c", "echo mine\n", "composer", "install"}
	if len(*got) < len(want) || !slices.Equal((*got)[len(*got)-len(want):], want) {
		t.Errorf("docker %q", *got)
	}
}
