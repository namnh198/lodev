package commands

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"lodev/internal/atomicfile"
	"lodev/internal/templates"
)

//go:embed shipped
var shippedFS embed.FS

// Shipped is the command files embedded in lodev (Path "", Marker true), sorted by Kind then Name.
func Shipped() []Command {
	var cmds []Command
	for _, kind := range []string{Host, Web} {
		entries, _ := shippedFS.ReadDir(path.Join("shipped", kind))
		for _, e := range entries {
			data, _ := shippedFS.ReadFile(path.Join("shipped", kind, e.Name()))
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
		if old, err := os.ReadFile(p); err == nil {
			if first, _, _ := strings.Cut(string(old), "\n"); first != templates.GeneratedMarker || string(old) == c.Script {
				continue
			}
		}
		if err := atomicfile.Write(p, []byte(c.Script), 0o644); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not write %s: %v", p, err))
		}
	}
	return warnings
}
