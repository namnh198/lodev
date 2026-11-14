package cmd

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"runtime"

	"github.com/spf13/cobra"

	"lodev/assets"
	"lodev/internal/certs"
	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/dockertools"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

// Swapped in tests.
var (
	firstRun      = firstRunSetup
	dockerPing    = docker.Ping
	setupDownload = dockertools.Path
	setupCA       = certs.EnsureCA
	hasMkcert     = func() bool { _, err := exec.LookPath("mkcert"); return err == nil }
	setupOS       = runtime.GOOS
)

// skipsSetup: help, completion, version and self-upgrade need no lodev home.
func skipsSetup(c *cobra.Command) bool {
	for ; c != nil; c = c.Parent() {
		switch c.Name() {
		case "help", "completion", "__complete", "__completeNoDesc", "version", "self-upgrade":
			return true
		}
	}
	return false
}

// firstRunSetup writes lodev's defaults while ~/.lodev/config.yaml does not exist yet:
// the config, projects.json, the data folder, the pinned docker tools and (without mkcert) the CA.
// A step that fails only warns: the command still runs, and later commands retry on demand.
func firstRunSetup() {
	if _, err := os.Stat(config.GlobalPath()); !errors.Is(err, fs.ErrNotExist) {
		return
	}
	ui.Info("First run: setting up lodev in %s", config.Home())
	data, err := assets.FS.ReadFile("templates/global-config.yaml")
	if setupOS == "linux" { // macOS resolves .local through mDNS (slow); Linux reads /etc/hosts first
		data = regexp.MustCompile(`(?m)^tld: test\b`).ReplaceAll(data, []byte("tld: local"))
	}
	if err == nil {
		err = os.MkdirAll(config.Home(), 0o755)
	}
	if err == nil {
		err = os.WriteFile(config.GlobalPath(), data, 0o644)
	}
	if err != nil {
		ui.Warn("could not write %s: %v", config.GlobalPath(), err)
		return
	}
	ui.Success("✔ Created %s", config.GlobalPath())
	if reg, err := registry.Load(); err != nil || reg.Save() != nil {
		ui.Warn("could not write projects.json: %v", err)
	}
	if g, err := config.LoadGlobal(); err == nil {
		if err := os.MkdirAll(g.DataPath(), 0o755); err != nil {
			ui.Warn("could not create %s: %v", g.DataPath(), err)
		}
	}
	if runtime.GOOS != "windows" { // Windows uses the system docker compose/buildx
		for _, t := range []dockertools.Tool{dockertools.Compose, dockertools.Buildx} {
			if _, err := setupDownload(t); err != nil {
				ui.Warn("could not download %s (lodev retries when it needs it): %v", t.Name(), err)
			}
		}
	}
	switch {
	case hasMkcert():
	case dockerPing() != nil:
		ui.Info("Docker is not running: lodev creates its local CA on the first `lodev start`.")
	default:
		if err := setupCA(); err != nil {
			ui.Warn("could not create the local CA (lodev retries on start): %v", err)
		}
	}
}

// needsDocker lists the commands that cannot run without the Docker daemon.
var needsDocker = map[string]bool{
	"lodev start": true, "lodev stop": true, "lodev restart": true, "lodev remove": true,
	"lodev ssh": true, "lodev exec": true, "lodev logs": true, "lodev xdebug": true,
	"lodev php": true, "lodev composer": true, "lodev npm": true, "lodev node": true,
	"lodev pnpm": true, "lodev yarn": true, "lodev grunt": true,
	"lodev artisan": true, "lodev wp": true, "lodev magento": true, "lodev n98": true,
	"lodev service start": true, "lodev service stop": true, "lodev service status": true,
	"lodev service logs": true, "lodev service enable": true, "lodev service disable": true,
	"lodev poweroff": true, "lodev clean": true, "lodev snapshot create": true, "lodev snapshot restore": true, "lodev db-export": true, "lodev db-import": true,
}
