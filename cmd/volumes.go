package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"lodev/internal/config"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

// projectVolumes is the names of the volumes labelled for the project, whatever the current config says.
func projectVolumes(name string) ([]string, error) {
	out, err := dockerOutput("volume", "ls", "-q", "-f", "label=lodev.project="+name)
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// ensureVolumes creates the project's performance_mode volumes that do not exist yet, copying the
// host folder into the ones that seed, and returns what compose mounts. img is the project image.
func ensureVolumes(p *config.Project, g *config.Global, img string) ([]templates.Volume, error) {
	var vols []templates.Volume
	var report []string
	for _, d := range p.VolumeDirsFor(g) {
		host := filepath.Join(p.Root, filepath.FromSlash(d.Path))
		fi, err := os.Lstat(host)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			ui.Warn("%s is a symlink on the host; not putting it in a volume", d.Path)
			continue
		}
		isDir := err == nil && fi.IsDir()
		v := templates.Volume{Name: config.VolumeName(p.Name, d.Path), Dir: d.Path}
		vols = append(vols, v)
		if _, err := dockerOutput("volume", "inspect", v.Name); err == nil {
			continue
		}
		if _, err := dockerOutput("volume", "create", "--label", "lodev.project="+p.Name, "--label", "lodev.dir="+d.Path, v.Name); err != nil {
			return nil, err
		}
		if d.Seed && isDir {
			// The image's entrypoint would start supervisord: run cp itself, as root to keep the host's owners.
			if _, err := dockerOutput("run", "--rm", "--user", "0", "--entrypoint", "cp",
				"-v", host+":/from:ro", "-v", v.Name+":/to", img, "-a", "/from/.", "/to/"); err != nil {
				// Remove it so the next start retries the copy instead of keeping an empty volume.
				dockerOutput("volume", "rm", v.Name)
				return nil, fmt.Errorf("copy %s into volume %s: %w", d.Path, v.Name, err)
			}
			report = append(report, d.Path+": copied from the host")
			continue
		}
		report = append(report, d.Path+": starts empty (host files are untouched, the container no longer sees them)")
	}
	if len(report) > 0 {
		for _, l := range report {
			ui.Info("%s", l)
		}
		hint := []string{"run lodev npm install"}
		if p.Type == "magento" {
			hint = append(hint, "lodev magento setup:di:compile if you need generated code up front")
		}
		hint = append(hint, "lodev config --performance-mode=none turns it off")
		if len(p.WebExtraDaemons) > 0 {
			hint = append(hint, "daemons retry on their own, lodev restart forces it")
		}
		ui.Info("hint: %s", strings.Join(hint, "; "))
	}
	have, err := projectVolumes(p.Name)
	if err != nil {
		return nil, err
	}
	var unused []string
	for _, n := range have {
		if !slices.ContainsFunc(vols, func(v templates.Volume) bool { return v.Name == n }) {
			unused = append(unused, n)
		}
	}
	if len(unused) > 0 {
		ui.Info("unused volumes: %s (lodev remove --volumes, or docker volume rm …)", strings.Join(unused, ", "))
	}
	return vols, nil
}

// removeVolumes keeps the project's labelled volumes and says how to drop them, or, with drop, deletes them after asking.
func removeVolumes(name string, drop, yes bool) error {
	vols, err := projectVolumes(name)
	if err != nil || len(vols) == 0 {
		return err
	}
	list := strings.Join(vols, ", ")
	if !drop {
		ui.Info("kept volumes: %s (lodev remove --volumes deletes them)", list)
		return nil
	}
	if !confirm(fmt.Sprintf("Delete volumes %s?", list), yes) {
		ui.Info("kept volumes: %s", list)
		return nil
	}
	for _, v := range vols {
		if _, err := dockerOutput("volume", "rm", v); err != nil {
			return err
		}
	}
	ui.Success("Deleted volumes %s", list)
	return nil
}
