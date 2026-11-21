package commands

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// runShipped runs the embedded host script name with bash, env added, and fakes (name → shell
// body) as executables first on PATH. Every fake also appends "<name> <args>" to $CALLS.
func runShipped(t *testing.T, name string, env map[string]string, fakes map[string]string, args ...string) (out string, calls []string, code int) {
	t.Helper()
	dir, callsFile := t.TempDir(), filepath.Join(t.TempDir(), "calls")
	for n, body := range fakes {
		script := "#!/bin/sh\necho \"" + n + " $*\" >> \"$CALLS\"\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var script string
	for _, c := range Shipped() {
		if c.Kind == Host && c.Name == name {
			script = c.Script
		}
	}
	if script == "" {
		t.Fatalf("no shipped host command %q", name)
	}
	x := exec.Command("bash", append([]string{"-c", script, name}, args...)...)
	x.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+callsFile)
	for k, v := range env {
		x.Env = append(x.Env, k+"="+v)
	}
	b, err := x.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(callsFile); err == nil {
		calls = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
	return string(b), calls, code
}

func TestOpenScript(t *testing.T) {
	env := map[string]string{"LODEV_BIN": "lodev"}
	fakes := func(osName, openBody string) map[string]string {
		return map[string]string{"lodev": "echo https://shop.test", "uname": "echo " + osName, "open": openBody, "xdg-open": openBody}
	}
	// without drops uname's own call, which every fake records.
	without := func(calls []string) []string {
		return slices.DeleteFunc(calls, func(c string) bool { return strings.HasPrefix(c, "uname ") })
	}
	for _, c := range []struct{ os, want string }{{"Darwin", "open https://shop.test"}, {"Linux", "xdg-open https://shop.test"}} {
		out, calls, code := runShipped(t, "open", env, fakes(c.os, "true"))
		if got := without(calls); code != 0 || out != "" || !slices.Equal(got, []string{"lodev url", c.want}) {
			t.Errorf("%s: %q %q %d", c.os, out, calls, code)
		}
	}

	out, calls, code := runShipped(t, "open", env, fakes("Darwin", "true"), "--print", "blog")
	if code != 0 || out != "https://shop.test\n" || !slices.Equal(calls, []string{"lodev url blog"}) {
		t.Errorf("--print: %q %q %d", out, calls, code)
	}

	out, calls, code = runShipped(t, "open", env, fakes("Darwin", "exit 1"), "blog")
	if code != 0 || !strings.Contains(out, "https://shop.test") || !strings.Contains(out, "could not open a browser") || !slices.Contains(calls, "open https://shop.test") {
		t.Errorf("failing open: %q %q %d", out, calls, code)
	}

	f := fakes("Darwin", "true")
	f["lodev"] = "exit 1"
	out, calls, code = runShipped(t, "open", env, f)
	if code != 1 || slices.Contains(calls, "open https://shop.test") {
		t.Errorf("failing lodev: %q %q %d", out, calls, code)
	}
}
