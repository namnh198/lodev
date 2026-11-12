package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"lodev/internal/config"
)

// exitStatus stands in for *exec.ExitError: a step that exited with a code.
type exitStatus int

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e exitStatus) ExitCode() int { return int(e) }

// stubHookExec records each command the hooks exec in the web container; fail maps the
// command's last word to an error.
func stubHookExec(t *testing.T, running bool, fail map[string]error) *[][]string {
	t.Helper()
	oldRun, oldUp, oldReady := dockerRun, containerRunning, userReady
	t.Cleanup(func() { dockerRun, containerRunning, userReady = oldRun, oldUp, oldReady })
	userReady = func(*config.Project, string) bool { return true }
	containerRunning = func(string) bool { return running }
	var got [][]string
	dockerRun = func(a ...string) error {
		i := slices.Index(a, "env")
		if i < 0 {
			return nil // not a hook step (compose stop etc.)
		}
		got = append(got, a[i:])
		return fail[a[len(a)-1]]
	}
	return &got
}

func hookProject(t *testing.T, hooks map[string][]config.HookStep) *config.Project {
	t.Helper()
	return &config.Project{Name: "shop", Type: "php", PHP: "8.4", Webserver: "nginx", Root: t.TempDir(), Hooks: hooks}
}

func writeHookFile(t *testing.T, p *config.Project, rel string, mode os.FileMode) {
	t.Helper()
	f := filepath.Join(p.Root, rel)
	os.MkdirAll(filepath.Dir(f), 0o755)
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func env(event string, cmd ...string) []string {
	return append([]string{"env", "LODEV_HOOK=" + event, "LODEV_PROJECT=shop"}, cmd...)
}

func TestHookFolderSteps(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Run: "composer install"}}})
	d := ".lodev/hooks/post-start.d/"
	writeHookFile(t, p, d+"20-b.sh", 0o755)
	writeHookFile(t, p, d+"10-a.sh", 0o755)
	writeHookFile(t, p, d+".hidden", 0o755)
	writeHookFile(t, p, d+"x.sh.example", 0o755)
	os.MkdirAll(filepath.Join(p.Root, d, "sub"), 0o755)
	writeHookFile(t, p, ".lodev/real.sh", 0o755)
	os.Symlink(filepath.Join(p.Root, ".lodev/real.sh"), filepath.Join(p.Root, d, "30-link.sh"))
	got := stubHookExec(t, true, nil)
	if err := runHooks(p, "post-start"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		env("post-start", "sh", "-c", "composer install"),
		env("post-start", "/var/www/html/"+d+"10-a.sh"),
		env("post-start", "/var/www/html/"+d+"20-b.sh"),
		env("post-start", "/var/www/html/"+d+"30-link.sh"),
	}
	if !slices.EqualFunc(*got, want, slices.Equal) {
		t.Errorf("steps:\n%q\nwant\n%q", *got, want)
	}
}

func TestHookCommandPassedVerbatim(t *testing.T) {
	cmd := `echo "a b" $X; true`
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Run: cmd}}})
	got := stubHookExec(t, true, nil)
	if err := runHooks(p, "post-start"); err != nil {
		t.Fatal(err)
	}
	if want := env("post-start", "sh", "-c", cmd); len(*got) != 1 || !slices.Equal((*got)[0], want) {
		t.Errorf("got %q", *got)
	}
}

func TestHookScriptChecks(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Script: ".lodev/hooks/x.sh"}}})
	stubHookExec(t, true, nil)
	if err := runHooks(p, "post-start"); err == nil || err.Error() != "hook post-start: .lodev/hooks/x.sh does not exist" {
		t.Errorf("missing: %v", err)
	}
	writeHookFile(t, p, ".lodev/hooks/x.sh", 0o644)
	want := "hook post-start: .lodev/hooks/x.sh is not executable (chmod +x .lodev/hooks/x.sh)"
	if err := runHooks(p, "post-start"); err == nil || err.Error() != want {
		t.Errorf("not executable: %v", err)
	}
	os.Chmod(filepath.Join(p.Root, ".lodev/hooks/x.sh"), 0o755)
	got := stubHookExec(t, true, nil)
	if err := runHooks(p, "post-start"); err != nil || !slices.Equal((*got)[0], env("post-start", "/var/www/html/.lodev/hooks/x.sh")) {
		t.Errorf("script run %q, %v", *got, err)
	}
}

func TestHookFailureStops(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Run: "one"}, {Run: "two"}, {Run: "three"}}})
	got := stubHookExec(t, true, map[string]error{"two": exitStatus(3)})
	err := runHooks(p, "post-start")
	if err == nil || err.Error() != "hook post-start failed: two: exit status 3" {
		t.Errorf("err = %v", err)
	}
	if len(*got) != 2 {
		t.Errorf("ran %q after the failure", *got)
	}
}

func TestHookWebNotRunning(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Run: "x"}}})
	stubHookExec(t, false, nil)
	if err := runHooks(p, "post-start"); err == nil || err.Error() != "hook post-start: lodev-shop-web is not running" {
		t.Errorf("err = %v", err)
	}
	if err := runHooks(hookProject(t, nil), "post-start"); err != nil {
		t.Errorf("no hooks must not need the container: %v", err)
	}
}

func TestIsProjectDB(t *testing.T) {
	p := &config.Project{Name: "shop", Services: []string{"mysql", "mongodb"}}
	for _, c := range []struct {
		t    dbTarget
		want bool
	}{{dbTarget{"mysql", "shop"}, true}, {dbTarget{"mongodb", "shop"}, true}, {dbTarget{"mysql", "other"}, false}, {dbTarget{"mariadb", "shop"}, false}} {
		if got := isProjectDB(p, c.t); got != c.want {
			t.Errorf("%+v: %v", c.t, got)
		}
	}
	if !strings.Contains(fmt.Sprint(p.Services), "mongodb") {
		t.Fatal("fixture")
	}
}

// runCmdWithHooks runs a root command in the shop project with hooks in its config and records
// docker calls: hook steps by their last word, compose calls as "compose <sub>".
func runCmdWithHooks(t *testing.T, hooksYAML string, fail map[string]error, args ...string) ([]string, error) {
	t.Helper()
	root := newShop(t)
	writeShop(t, root, shopConf+hooksYAML)
	oldRun, oldUp, oldReady := dockerRun, containerRunning, userReady
	t.Cleanup(func() { dockerRun, containerRunning, userReady = oldRun, oldUp, oldReady })
	userReady = func(*config.Project, string) bool { return true }
	containerRunning = func(string) bool { return true }
	var calls []string
	dockerRun = func(a ...string) error {
		last := a[len(a)-1]
		if slices.Contains(a, "env") {
			calls = append(calls, "hook "+last)
			return fail[last]
		}
		calls = append(calls, "docker "+strings.Join(a[len(a)-1:], " "))
		return nil
	}
	c, _, _ := rootCmd.Find(args[:1])
	t.Cleanup(func() {
		c.Flags().VisitAll(func(f *pflag.Flag) { f.Value.Set(f.DefValue); f.Changed = false })
		rootCmd.SetArgs(nil)
	})
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return calls, err
}

func TestStopRunsPreStop(t *testing.T) {
	hooks := "hooks:\n  pre-stop: [\"php bin/console down\"]\n"
	calls, err := runCmdWithHooks(t, hooks, nil, "stop")
	if err != nil || !slices.Equal(calls, []string{"hook php bin/console down", "docker stop"}) {
		t.Errorf("stop: %q %v", calls, err)
	}
	calls, err = runCmdWithHooks(t, hooks, map[string]error{"php bin/console down": exitStatus(1)}, "stop")
	if err == nil || !strings.HasSuffix(err.Error(), "(lodev stop --no-hooks stops anyway)") || slices.Contains(calls, "docker stop") {
		t.Errorf("failing pre-stop: %q %v", calls, err)
	}
	calls, err = runCmdWithHooks(t, hooks, nil, "stop", "--no-hooks")
	if err != nil || !slices.Equal(calls, []string{"docker stop"}) {
		t.Errorf("--no-hooks: %q %v", calls, err)
	}
}

func TestDBImportPostHook(t *testing.T) {
	root := newShop(t)
	writeShop(t, root, shopConf+"hooks:\n  post-db-import: [\"echo imported\"]\n")
	stubDBStream(t, "")
	ran := stubHookExec(t, true, nil)
	f := filepath.Join(t.TempDir(), "d.sql")
	os.WriteFile(f, []byte("SELECT 1;\n"), 0o644)
	if _, _, err := runConfig(t, newDBImportCmd, "-f", f); err != nil || len(*ran) != 1 {
		t.Errorf("own db: %q %v", *ran, err)
	}
	if _, _, err := runConfig(t, newDBImportCmd, "other", "-f", f); err != nil || len(*ran) != 1 {
		t.Errorf("other db must not run the hook: %q %v", *ran, err)
	}
	if _, _, err := runConfig(t, newDBImportCmd, "-f", f, "--no-hooks"); err != nil || len(*ran) != 1 {
		t.Errorf("--no-hooks: %q %v", *ran, err)
	}
}
