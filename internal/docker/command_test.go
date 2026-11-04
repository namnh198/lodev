package docker

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/dockertools"
	"lodev/internal/ui"
)

type commandFix struct {
	pathCalls int
	pathErr   error
	sysVer    string
	global    *config.Global
	globalErr error
}

// stubCommand swaps command's seams: managed binaries live in /home/bin.
func stubCommand(t *testing.T) *commandFix {
	t.Helper()
	f := &commandFix{global: &config.Global{}}
	oldPath, oldSys, oldLoad, oldOS := toolPath, systemVersion, loadGlobal, goos
	t.Cleanup(func() { toolPath, systemVersion, loadGlobal, goos = oldPath, oldSys, oldLoad, oldOS; resetResolved() })
	resetResolved()
	toolPath = func(tool dockertools.Tool) (string, error) {
		f.pathCalls++
		if f.pathErr != nil {
			return "", f.pathErr
		}
		return "/home/bin/" + tool.Name(), nil
	}
	systemVersion = func(string) string { return f.sysVer }
	loadGlobal = func() (*config.Global, error) { return f.global, f.globalErr }
	goos = "darwin"
	return f
}

func mustCommand(t *testing.T, args ...string) (string, []string) {
	t.Helper()
	c, err := command(args)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Base(c.Path), c.Args[1:]
}

func expect(t *testing.T, args []string, prog string, want ...string) {
	t.Helper()
	gotProg, got := mustCommand(t, args...)
	if gotProg != prog || !slices.Equal(got, want) {
		t.Errorf("%v → %s %v, want %s %v", args, gotProg, got, prog, want)
	}
}

func TestCommandManaged(t *testing.T) {
	stubCommand(t)
	expect(t, []string{"compose", "-p", "x", "up"}, "docker-compose", "-p", "x", "up")
	// --load: the image reaches the daemon even when the user's builder is a docker-container one.
	expect(t, []string{"build", "-t", "a", "."}, "docker-buildx", "build", "--load", "-t", "a", ".")
	expect(t, []string{"info"}, "docker", "info")
}

func TestCommandSystemKeys(t *testing.T) {
	f := stubCommand(t)
	f.global.UseDockerComposeSystem, f.global.UseDockerBuildxSystem = true, true
	expect(t, []string{"compose", "-p", "x", "up"}, "docker", "compose", "-p", "x", "up")
	expect(t, []string{"build", "-t", "a", "."}, "docker", "build", "-t", "a", ".")
	if f.pathCalls != 0 {
		t.Errorf("downloaded in system mode: %d", f.pathCalls)
	}
}

func TestCommandFallback(t *testing.T) {
	f := stubCommand(t)
	f.pathErr, f.sysVer = errors.New("offline"), "v5.5.0"
	_, errb, restore := ui.Capture(false)
	defer restore()
	expect(t, []string{"compose", "up"}, "docker", "compose", "up")
	expect(t, []string{"compose", "ps"}, "docker", "compose", "ps")
	want := "could not download docker-compose v5.7.0 (offline); using the system one (v5.5.0)"
	if strings.Count(errb.String(), want) != 1 || f.pathCalls != 1 {
		t.Errorf("warnings %q, download attempts %d", errb.String(), f.pathCalls)
	}
}

func TestCommandNoTool(t *testing.T) {
	f := stubCommand(t)
	f.pathErr = errors.New("offline")
	_, err := command([]string{"compose", "up"})
	if err == nil || err.Error() != "docker-compose v5.7.0 could not be downloaded (offline) and docker compose is not installed" {
		t.Errorf("err = %v", err)
	}
	_, err = command([]string{"build", "."})
	if err == nil || err.Error() != "docker-buildx v0.38.2 could not be downloaded (offline) and docker buildx is not installed" {
		t.Errorf("err = %v", err)
	}
}

// A broken config must not stop `lodev config-global` from fixing it.
func TestCommandBrokenConfig(t *testing.T) {
	f := stubCommand(t)
	f.global, f.globalErr = nil, errors.New("bad yaml")
	expect(t, []string{"compose", "up"}, "docker-compose", "up")
}

func TestCommandWindows(t *testing.T) {
	f := stubCommand(t)
	goos = "windows"
	expect(t, []string{"compose", "up"}, "docker", "compose", "up")
	if f.pathCalls != 0 {
		t.Errorf("downloaded on windows")
	}
}
