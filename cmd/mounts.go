package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"lodev/internal/config"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

// prepareMounts readies p's web mounts for a start: it migrates an older lodev's home, takes the
// trust-gated copies of homeadditional and the apache config, and creates the performance_mode
// volumes. img is the project image, which seeds the volumes.
func prepareMounts(g *config.Global, p *config.Project, img string) (templates.Mounts, error) {
	// Created here, not by Docker: Docker would create a missing bind source owned by root.
	home := projectHome(g, p)
	if err := migrateHome(g, p); err != nil {
		return templates.Mounts{}, err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return templates.Mounts{}, err
	}
	warnHomeMigration(home)
	globalAdd := filepath.Join(config.Home(), "homeadditional")
	if err := os.MkdirAll(globalAdd, 0o755); err != nil {
		return templates.Mounts{}, err
	}
	projectAdd, err := snapshotHomeAdditional(p, projectHomeAdditional(g, p))
	if err != nil {
		return templates.Mounts{}, err
	}
	apacheSnap, err := snapshotApache(p, projectApacheSnapshot(g, p))
	if err != nil {
		return templates.Mounts{}, err
	}
	vols, err := ensureVolumes(p, g, img)
	if err != nil {
		return templates.Mounts{}, err
	}
	return templates.Mounts{HomeDir: home, GlobalHomeAdditional: globalAdd, ProjectHomeAdditional: projectAdd, ApacheSnapshot: apacheSnap, Volumes: vols}, nil
}

// removeMounts drops p's web mounts when the project is removed: the copies go, the home (caches
// and histories) stays, and the volumes go only with dropVolumes, after asking.
func removeMounts(p *config.Project, dropVolumes, yes bool) error {
	if g, err := config.LoadGlobal(); err == nil {
		h := projectHome(g, p)
		if _, err := os.Stat(h); err == nil {
			ui.Info("Kept %s (caches and histories)", h)
		}
		for _, snap := range []string{projectHomeAdditional(g, p), projectApacheSnapshot(g, p)} {
			if err := os.RemoveAll(snap); err != nil {
				return err
			}
		}
	}
	return removeVolumes(p.Name, dropVolumes, yes)
}

// projectHome is p's data folder, mounted at /var/lib/lodev/home: the container's caches and
// histories, plus the homeadditional/ and apache/ copies start takes.
func projectHome(g *config.Global, p *config.Project) string {
	return filepath.Join(g.DataPath(), "web", p.Name+"-web")
}

// projectHomeAdditional is the copy of p's .lodev/homeadditional the web container mounts.
func projectHomeAdditional(g *config.Global, p *config.Project) string {
	return filepath.Join(projectHome(g, p), "homeadditional")
}

// migrateHome moves an older lodev's <data>/home-<name> to projectHome (left alone, with a warning,
// when both exist) and drops its homeadditional-<name> and apache-<name> copies, which start retakes.
func migrateHome(g *config.Global, p *config.Project) error {
	old, home := filepath.Join(g.DataPath(), "home-"+p.Name), projectHome(g, p)
	if _, err := os.Stat(old); err == nil {
		if _, err := os.Stat(home); err == nil {
			ui.Warn("%s is no longer used (lodev now uses %s); move what you need and delete it", old, home)
		} else {
			if err := os.MkdirAll(filepath.Dir(home), 0o755); err != nil {
				return err
			}
			if err := os.Rename(old, home); err != nil {
				return err
			}
		}
	}
	for _, d := range []string{"homeadditional-", "apache-"} {
		if err := os.RemoveAll(filepath.Join(g.DataPath(), d+p.Name)); err != nil {
			return err
		}
	}
	return nil
}

// snapshotHomeAdditional replaces dst's contents with a copy of p's .lodev/homeadditional (a
// symlinked folder followed; regular files and symlinks, as trustItems lists them) and returns dst.
// Mounting the copy, not the repo folder, keeps a restart by Docker (restart: unless-stopped) after
// a git pull from copying files nobody approved. No folder: dst is removed and "" returned.
func snapshotHomeAdditional(p *config.Project, dst string) (string, error) {
	src := filepath.Join(p.Root, ".lodev", "homeadditional")
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return "", os.RemoveAll(dst)
	}
	return dst, snapshotTree(src, dst, true)
}

// projectApacheSnapshot is the copy of p's apache config (webserver.conf and custom.d/) the web
// container mounts.
func projectApacheSnapshot(g *config.Global, p *config.Project) string {
	return filepath.Join(projectHome(g, p), "apache")
}

// snapshotApache copies p's .lodev/apache-webserver.conf and .lodev/apache/ into dst as
// webserver.conf and custom.d/ and returns dst; apache runs commands from its config (piped logs),
// so like homeadditional the container must only see what the trust gate approved. Not apache:
// dst is removed and "" returned (nginx's config cannot run commands; it stays mounted from the repo).
func snapshotApache(p *config.Project, dst string) (string, error) {
	if p.Webserver != "apache" {
		return "", os.RemoveAll(dst)
	}
	conf, err := os.ReadFile(WebserverConfFile(p))
	if err != nil {
		return "", err
	}
	custom := filepath.Join(dst, "custom.d")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		return "", err
	}
	// WriteFile truncates in place: the inode a running container bind-mounts stays the same.
	if err := os.WriteFile(filepath.Join(dst, "webserver.conf"), conf, 0o644); err != nil {
		return "", err
	}
	return dst, snapshotTree(filepath.Join(p.Root, ".lodev", "apache"), custom, false)
}

// snapshotTree replaces dst's contents with a copy of src (a symlinked src followed; regular files
// and symlinks, as trustItems lists them; skipExamples leaves out *.example files). A missing src
// leaves dst empty.
func snapshotTree(src, dst string, skipExamples bool) error {
	// Contents only: a running container bind-mounts dst, and a new inode would leave it on a deleted folder.
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	old, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, e := range old {
		if err := os.RemoveAll(filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return walkTree(src, func(path, rel string, d fs.DirEntry) error {
		if skipExamples && !d.IsDir() && strings.HasSuffix(rel, ".example") {
			return nil
		}
		to := filepath.Join(dst, filepath.FromSlash(rel))
		fi, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.Mkdir(to, fi.Mode().Perm()|0o700) // writable, so the next start can clear it
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(t, to)
		case d.Type().IsRegular():
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(to, b, fi.Mode().Perm())
		}
		return nil
	})
}

// homeMarker says the project's home was seen by a lodev that mounts it as caches only.
const homeMarker = ".lodev-home-v2"

// warnHomeMigration lists, once per home folder, what older lodevs kept there that the container's
// home no longer shows. Nothing is moved or deleted.
func warnHomeMigration(home string) {
	marker := filepath.Join(home, homeMarker)
	if _, err := os.Stat(marker); err == nil {
		return
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		ui.Warn("%v", err)
		return // no marker: the warning is not lost
	}
	var left []string
	for _, e := range entries {
		switch e.Name() {
		case ".bash_history", ".mysql_history", ".npm", ".cache", ".local", "homeadditional", "apache":
		case ".config": // psysh's history lives here; the rest (composer auth...) is not kept
			subs, _ := os.ReadDir(filepath.Join(home, ".config"))
			for _, c := range subs {
				if c.Name() != "psysh" {
					left = append(left, ".config/"+c.Name())
				}
			}
		default:
			left = append(left, e.Name())
		}
	}
	slices.Sort(left)
	if len(left) > 0 {
		ui.Warn("these files in %s are no longer in the container's home: %s; move the ones you need to ~/.lodev/homeadditional", home, strings.Join(left, ", "))
	}
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		ui.Warn("%v", err)
	}
}
