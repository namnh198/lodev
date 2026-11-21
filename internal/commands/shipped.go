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
)

// assets is where the shipped files come from; swapped in tests.
var assets fs.FS = dotlodev_assets.FS

// Shipped is the command files embedded in lodev (Path "", Marker true), sorted by Kind then
// Name, each with its embedded commands/<kind>/autocomplete/<name> when there is one.
func Shipped() []Command {
	var cmds []Command
	for _, kind := range []string{Host, Web} {
		dir := path.Join("commands", kind)
		entries, _ := fs.ReadDir(assets, dir)
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(e.Name(), ".example") { // scaffolded samples, not shipped commands
				continue
			}
			data, _ := fs.ReadFile(assets, path.Join(dir, e.Name()))
			if c, err := Parse(e.Name(), kind, "", data); err == nil { // a bad embedded file fails TestShippedWeb
				c.Marker = true
				if data, err := fs.ReadFile(assets, path.Join(dir, "autocomplete", e.Name())); err == nil {
					if ac, err := Parse(e.Name(), kind, "", data); err == nil {
						ac.Marker = true
						c.Complete = &ac
					}
				}
				cmds = append(cmds, c)
			}
		}
	}
	return cmds
}

// Sync writes the shipped files, and their autocomplete scripts, to <home>/commands/<kind>/
// when missing, or when they carry the marker and differ. A file without the marker is the
// user's and is never touched.
func Sync(home string) (warnings []string) {
	for _, c := range Shipped() {
		dir := filepath.Join(home, "commands", c.Kind)
		warnings = append(warnings, syncFile(filepath.Join(dir, c.Name), c.Script)...)
		if c.Complete != nil {
			warnings = append(warnings, syncFile(filepath.Join(dir, "autocomplete", c.Name), c.Complete.Script)...)
		}
	}
	return warnings
}

func syncFile(p, script string) []string {
	if fi, err := os.Lstat(p); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil // never replace a user's symlink
		}
		old, err := os.ReadFile(p)
		if err != nil {
			return []string{fmt.Sprintf("could not read %s, left as is: %v", p, err)}
		}
		if !HasMarker(old) || string(old) == script {
			return nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return []string{fmt.Sprintf("could not check %s, left as is: %v", p, err)}
	}
	if err := atomicfile.Write(p, []byte(script), 0o644); err != nil {
		return []string{fmt.Sprintf("could not write %s: %v", p, err)}
	}
	return nil
}
