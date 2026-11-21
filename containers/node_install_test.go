package containers

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nodeInstallSkip = "lodev: dependencies unchanged since the last install; skipping pnpm install --frozen-lockfile"

// nodeInstallProject is a project folder with package.json and a pnpm lockfile, plus a stub bin
// whose pnpm logs each call to calls and fills node_modules, and whose node prints $NODE_V.
func nodeInstallProject(t *testing.T) (dir, bin, calls string) {
	t.Helper()
	if _, err := exec.LookPath("md5sum"); err != nil {
		t.Skip("no md5sum")
	}
	dir, bin = t.TempDir(), t.TempDir()
	calls = filepath.Join(bin, "calls")
	for name, body := range map[string]string{
		"pnpm": `echo "$*" >> "$CALLS"
[ -n "$NO_MODULES" ] || { mkdir -p node_modules && echo "$MODULES" > node_modules/.modules.yaml; }
exit "${FAIL:-0}"`,
		"node": `echo "${NODE_V:-v24.0.0}"`,
	} {
		writeTestFile(t, filepath.Join(bin, name), "#!/bin/sh\n"+body+"\n", 0o755)
	}
	writeTestFile(t, filepath.Join(dir, "package.json"), "{}\n", 0o644)
	writeTestFile(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: 9\n", 0o644)
	return dir, bin, calls
}

func writeTestFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// runNodeInstall runs webserver/node-install.sh pnpm install --frozen-lockfile in dir.
func runNodeInstall(t *testing.T, dir, bin, calls string, env ...string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("webserver/node-install.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script, "pnpm", "install", "--frozen-lockfile")
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + bin + ":" + os.Getenv("PATH"), "CALLS=" + calls, "MODULES=container"}, env...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func installs(t *testing.T, calls string) int {
	t.Helper()
	b, err := os.ReadFile(calls)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

func stampExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "node_modules", ".lodev-install"))
	return err == nil
}

func TestNodeInstallSkipsUnchanged(t *testing.T) {
	dir, bin, calls := nodeInstallProject(t)
	if out, err := runNodeInstall(t, dir, bin, calls); err != nil || installs(t, calls) != 1 || !stampExists(dir) {
		t.Fatalf("first run: %q %v, %d installs, stamp %v", out, err, installs(t, calls), stampExists(dir))
	}
	out, err := runNodeInstall(t, dir, bin, calls)
	if err != nil || out != nodeInstallSkip || installs(t, calls) != 1 {
		t.Errorf("second run: %q %v, %d installs", out, err, installs(t, calls))
	}
}

// Anything that would leave stale or wrong packages installs again.
func TestNodeInstallReinstallsOnChange(t *testing.T) {
	for name, change := range map[string]func(dir string) []string{
		"lockfile": func(dir string) []string {
			writeTestFile(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: 9\nchanged: 1\n", 0o644)
			return nil
		},
		"package.json": func(dir string) []string {
			writeTestFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"next":"16"}}`, 0o644)
			return nil
		},
		"pnpm-workspace.yaml": func(dir string) []string {
			writeTestFile(t, filepath.Join(dir, "pnpm-workspace.yaml"), "allowBuilds:\n  sharp: true\n", 0o644)
			return nil
		},
		"node version": func(string) []string { return []string{"NODE_V=v22.0.0"} },
		"host install": func(dir string) []string {
			writeTestFile(t, filepath.Join(dir, "node_modules", ".modules.yaml"), "host\n", 0o644)
			return nil
		},
		"node_modules deleted": func(dir string) []string {
			if err := os.RemoveAll(filepath.Join(dir, "node_modules")); err != nil {
				t.Fatal(err)
			}
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, bin, calls := nodeInstallProject(t)
			if out, err := runNodeInstall(t, dir, bin, calls); err != nil {
				t.Fatalf("first run: %q %v", out, err)
			}
			env := change(dir)
			if out, err := runNodeInstall(t, dir, bin, calls, env...); err != nil || installs(t, calls) != 2 {
				t.Errorf("after change: %q %v, %d installs", out, err, installs(t, calls))
			}
		})
	}
}

func TestNodeInstallFailureWritesNoStamp(t *testing.T) {
	dir, bin, calls := nodeInstallProject(t)
	_, err := runNodeInstall(t, dir, bin, calls, "FAIL=3")
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || stampExists(dir) {
		t.Fatalf("failed install: %v, stamp %v", err, stampExists(dir))
	}
	if out, err := runNodeInstall(t, dir, bin, calls); err != nil || installs(t, calls) != 2 || !stampExists(dir) {
		t.Errorf("retry: %q %v, %d installs, stamp %v", out, err, installs(t, calls), stampExists(dir))
	}
}

// npm with no dependencies creates no node_modules; the stamp still records the install.
func TestNodeInstallNoNodeModules(t *testing.T) {
	dir, bin, calls := nodeInstallProject(t)
	if out, err := runNodeInstall(t, dir, bin, calls, "NO_MODULES=1"); err != nil || !stampExists(dir) {
		t.Errorf("%q %v, stamp %v", out, err, stampExists(dir))
	}
}
