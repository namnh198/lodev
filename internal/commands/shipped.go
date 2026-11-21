package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"lodev/internal/atomicfile"
	"lodev/internal/dotlodev_assets"
	"lodev/internal/templates"
)

// Shipped is the command files embedded in lodev (Path "", Marker true), sorted by Kind then Name.
func Shipped() []Command {
	var cmds []Command
	for _, kind := range []string{Host, Web} {
		entries, _ := dotlodev_assets.FS.ReadDir(path.Join("commands", kind))
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".example") { // scaffolded samples, not shipped commands
				continue
			}
			data, _ := dotlodev_assets.FS.ReadFile(path.Join("commands", kind, e.Name()))
			if c, err := Parse(e.Name(), kind, "", data); err == nil { // a bad embedded file fails TestShippedWeb
				c.Marker = true
				cmds = append(cmds, c)
			}
		}
	}
	return cmds
}

// Sync writes the shipped files to <home>/commands/<kind>/<name> when missing, or when they
// carry the marker and differ. A file without the marker is the user's and is never touched.
func Sync(home string) (warnings []string) {
	for _, c := range Shipped() {
		p := filepath.Join(home, "commands", c.Kind, c.Name)
		if fi, err := os.Lstat(p); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				continue // never replace a user's symlink
			}
			old, err := os.ReadFile(p)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not read %s, left as is: %v", p, err))
				continue
			}
			if first, _, _ := strings.Cut(string(old), "\n"); first != templates.GeneratedMarker || string(old) == c.Script {
				continue
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			warnings = append(warnings, fmt.Sprintf("could not check %s, left as is: %v", p, err))
			continue
		}
		if err := atomicfile.Write(p, []byte(c.Script), 0o644); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not write %s: %v", p, err))
		}
	}
	return warnings
}
