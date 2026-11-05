package cmd

import (
	"fmt"
	"os"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// prepareServiceData creates the data folders of svcs (with mount_volume) and, into an empty folder,
// copies the service's old named volume once; the volume is kept.
func prepareServiceData(g *config.Global, svcs []string) error {
	for _, svc := range svcs {
		path := g.MountPath(svc)
		if path == "" {
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
		if svc == "opensearch" {
			// It runs as UID 1000 and never chowns its data dir (the others do).
			if err := os.Chmod(path, 0o777); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		vol := "lodev-" + svc
		if len(entries) > 0 {
			continue
		}
		// Inspect first: `docker run -v` would create a missing volume.
		if _, err := dockerOutput("volume", "inspect", vol); err != nil {
			continue
		}
		// The container is named like the volume; never copy files a running database is writing.
		if id, _ := dockerOutput("ps", "-aq", "--filter", "name=^"+vol+"$"); id != "" {
			if _, err := dockerOutput("stop", vol); err != nil {
				return err
			}
		}
		if err := dockerRun("run", "--rm", "--user", "0", "--entrypoint", "cp",
			"-v", vol+":/from", "-v", path+":/to", g.Image(svc), "-a", "/from/.", "/to/"); err != nil {
			return fmt.Errorf("copy volume %s to %s: %w (empty %s before retrying)", vol, path, err, path)
		}
		ui.Info("Copied volume %s to %s (the volume is kept)", vol, path)
	}
	return nil
}
