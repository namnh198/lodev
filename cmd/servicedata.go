package cmd

import (
	"errors"
	"fmt"
	"io/fs"
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
		_, statErr := os.Stat(path)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
		if svc == "opensearch" && errors.Is(statErr, fs.ErrNotExist) {
			// It runs as UID 1000 and never chowns its data dir (the others do). Only a folder lodev
			// just made: chmod fails on one opensearch already owns.
			if err := os.Chmod(path, 0o777); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(path)
		if errors.Is(err, fs.ErrPermission) {
			continue // owned by the service's UID since its first start (Linux): it has data
		}
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
