package docker

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"lodev/internal/config"
	"lodev/internal/dockertools"
	"lodev/internal/ui"
)

// Swapped in tests.
var (
	toolPath   = dockertools.Path
	loadGlobal = config.LoadGlobal
	goos       = runtime.GOOS
	// systemVersion is the system plugin's version ("v5.5.0"), or "" when it is not installed.
	systemVersion = func(sub string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		args := []string{sub, "version"}
		if sub == "compose" {
			args = append(args, "--short") // "5.5.0"; buildx: "github.com/docker/buildx v0.37.2 <commit>"
		}
		out, err := exec.CommandContext(ctx, "docker", args...).Output()
		if err != nil {
			return ""
		}
		if f := strings.Fields(string(out)); sub == "buildx" && len(f) > 1 {
			return f[1]
		} else if len(f) > 0 {
			return "v" + strings.TrimPrefix(f[0], "v")
		}
		return ""
	}
)

var (
	resolvedMu sync.Mutex
	// resolved caches, per process, the program for "compose" / "build": a managed binary or "docker".
	resolved = map[string]string{}
)

func resetResolved() {
	resolvedMu.Lock()
	defer resolvedMu.Unlock()
	resolved = map[string]string{}
}

// command runs compose calls with lodev's pinned docker-compose and builds with its docker-buildx,
// unless the global config asks for the system ones; everything else is plain docker.
func command(args []string) (*exec.Cmd, error) {
	if len(args) > 0 && goos != "windows" {
		switch args[0] {
		case "compose":
			p, err := program("compose", dockertools.Compose)
			if err != nil {
				return nil, err
			}
			if p != "docker" {
				return exec.Command(p, args[1:]...), nil
			}
		case "build":
			p, err := program("build", dockertools.Buildx)
			if err != nil {
				return nil, err
			}
			if p != "docker" {
				// --load: the image reaches the daemon even when the current builder is a docker-container one.
				return exec.Command(p, append([]string{"build", "--load"}, args[1:]...)...), nil
			}
		}
	}
	return exec.Command("docker", args...), nil
}

func program(kind string, t dockertools.Tool) (string, error) {
	resolvedMu.Lock()
	defer resolvedMu.Unlock()
	if p, ok := resolved[kind]; ok {
		return p, nil
	}
	// A config that does not load falls back to the managed default, so config-global can still fix it.
	g, err := loadGlobal()
	system := err == nil && (kind == "compose" && g.UseDockerComposeSystem || kind == "build" && g.UseDockerBuildxSystem)
	p := "docker"
	if !system {
		path, err := toolPath(t)
		if err != nil {
			sub := map[string]string{"compose": "compose", "build": "buildx"}[kind]
			v := systemVersion(sub)
			if v == "" {
				return "", fmt.Errorf("%s %s could not be downloaded (%v) and docker %s is not installed", t.Name(), t.Version(), err, sub)
			}
			ui.Warn("could not download %s %s (%v); using the system one (%s)", t.Name(), t.Version(), err, v)
		} else {
			p = path
		}
	}
	resolved[kind] = p
	return p, nil
}
