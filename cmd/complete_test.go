package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/ui"
)

// completeRoot runs `lodev __complete args...` and returns its stdout.
func completeRoot(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	defer rootCmd.SetOut(nil)
	if err := runRoot(t, append([]string{"__complete"}, args...)...); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

func TestCompleteHost(t *testing.T) {
	root := customProject(t, "php", true)
	withoutShipped(t)
	stubHostRun(t)
	writeFile(t, filepath.Join(root, ".lodev/commands/host/hi"), "echo hi\n")
	writeFile(t, filepath.Join(root, ".lodev/commands/host/autocomplete/hi"),
		"#!/bin/sh\nfor last; do :; done\n[ $# = 2 ] && echo \"$last=$#\"\necho 'a bc'\nprintf 'zzz\\tthe end\\n'\n")
	registerFor(t, root)
	if got := completeRoot(t, "hi", "x", "a b"); got != "a b=2\na bc\n:4" {
		t.Errorf("word \"a b\": %q", got)
	}
	if got := completeRoot(t, "hi", "x", ""); got != "=2\na bc\nzzz\tthe end\n:4" {
		t.Errorf("empty word: %q", got)
	}
	if got := completeRoot(t, "hi", "zq"); got != ":0" {
		t.Errorf("no match: %q", got)
	}
}

func TestCompleteHostFailure(t *testing.T) {
	root := customProject(t, "php", true)
	withoutShipped(t)
	stubHostRun(t)
	writeFile(t, filepath.Join(root, ".lodev/commands/host/hi"), "echo hi\n")
	writeFile(t, filepath.Join(root, ".lodev/commands/host/autocomplete/hi"), "#!/bin/sh\necho one\nexit 1\n")
	registerFor(t, root)
	if got := completeRoot(t, "hi", ""); got != ":0" {
		t.Errorf("failing script: %q", got)
	}
}

func TestCompleteTimeout(t *testing.T) {
	root := customProject(t, "php", true)
	withoutShipped(t)
	stubHostRun(t)
	old := completeTimeout
	t.Cleanup(func() { completeTimeout = old })
	completeTimeout = 200 * time.Millisecond
	writeFile(t, filepath.Join(root, ".lodev/commands/host/hi"), "echo hi\n")
	writeFile(t, filepath.Join(root, ".lodev/commands/host/autocomplete/hi"), "sleep 5\necho late\n")
	registerFor(t, root)
	start := time.Now()
	got := completeRoot(t, "hi", "")
	if d := time.Since(start); d > time.Second {
		t.Errorf("took %v", d)
	}
	if got != ":0" {
		t.Errorf("got %q", got)
	}
}

// stubWebComplete stubs a web container that is running or not, and records dockerOutput calls.
func stubWebComplete(t *testing.T, running bool, out string) *[][]string {
	t.Helper()
	oldOut, oldUp := dockerOutput, containerRunning
	t.Cleanup(func() { dockerOutput, containerRunning = oldOut, oldUp })
	containerRunning = func(string) bool { return running }
	var calls [][]string
	dockerOutput = func(a ...string) (string, error) { calls = append(calls, a); return out, nil }
	return &calls
}

func TestCompleteWebStopped(t *testing.T) {
	root := customProject(t, "php", true)
	withoutShipped(t)
	writeFile(t, filepath.Join(root, ".lodev/commands/web/composer"), "exec composer \"$@\"\n")
	writeFile(t, filepath.Join(root, ".lodev/commands/web/autocomplete/composer"), "echo require\n")
	calls := stubWebComplete(t, false, "require")
	registerFor(t, root)
	if got := completeRoot(t, "composer", ""); got != ":0" {
		t.Errorf("got %q", got)
	}
	if len(*calls) != 0 {
		t.Errorf("docker ran: %q", *calls)
	}
}

func TestCompleteWebArgv(t *testing.T) {
	root := customProject(t, "php", true)
	withoutShipped(t)
	script := "echo require\n"
	writeFile(t, filepath.Join(root, ".lodev/commands/web/composer"), "exec composer \"$@\"\n")
	writeFile(t, filepath.Join(root, ".lodev/commands/web/autocomplete/composer"), script)
	calls := stubWebComplete(t, true, "require\nremove\ninstall")
	registerFor(t, root)
	if got := completeRoot(t, "composer", "require", ""); got != "require\nremove\ninstall\n:4" {
		t.Errorf("got %q", got)
	}
	p, err := config.LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	user, _ := p.ContainerUser()
	want := docker.ExecArgs(p.ComposeProject(), composeFile(p), "web", user, p.WorkDir(), false,
		[]string{"bash", "-c", script, "composer", "require", ""})
	if len(*calls) != 1 || !slices.Equal((*calls)[0], want) {
		t.Errorf("docker %q\nwant %q", *calls, want)
	}
}

func TestCompleteQuiet(t *testing.T) {
	root := customProject(t, "php", true)
	withoutShipped(t)
	writeFile(t, filepath.Join(root, ".lodev/commands/web/Bad"), "echo\n")
	if _, e := registerFor(t, root); !strings.Contains(e, "warning") {
		t.Fatalf("no warning outside completion: %q", e)
	}
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	for _, first := range []string{"__complete", "__completeNoDesc"} {
		os.Args = []string{"lodev", first, ""}
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		o, e, restore := ui.Capture(false)
		err := Execute()
		restore()
		rootCmd.SetOut(nil)
		if err != nil {
			t.Fatal(err)
		}
		if all := out.String() + o.String() + e.String(); strings.Contains(all, "warning") {
			t.Errorf("%s printed %q", first, all)
		}
	}
}
