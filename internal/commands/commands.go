// Package commands parses and loads custom command files (bash scripts that become `lodev <name>`).
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"lodev/internal/config"
	"lodev/internal/templates"
)

const Web, Host = "web", "host"

type Command struct {
	Name, Kind string // Kind is Web or Host
	Path       string // file on disk; "" for an embedded shipped copy
	Script     string // file contents, CRLF → LF
	Use, Short string
	Types      []string // nil: every project type
	Marker     bool     // line 1 is templates.GeneratedMarker
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func ValidName(name string) bool { return nameRe.MatchString(name) }

// Parse reads the header (the leading run of `#` lines); only an unknown Type is an error.
func Parse(name, kind, path string, data []byte) (Command, error) {
	script := strings.ReplaceAll(string(data), "\r\n", "\n")
	c := Command{Name: name, Kind: kind, Path: path, Script: script}
	c.Use = name + " [args...]"
	c.Short = "Custom command (" + shortPath(path) + ")"
	for i, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(line, "#") {
			break
		}
		if i == 0 && line == templates.GeneratedMarker {
			c.Marker = true
		}
		key, val, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "#")), ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "Usage":
			if first, _, _ := strings.Cut(val, " "); first == name {
				c.Use = val
			}
		case "Description":
			if val != "" {
				c.Short = val
			}
		case "Type":
			for _, t := range strings.Split(val, ",") {
				t = strings.TrimSpace(t)
				if !slices.Contains(config.Types, t) {
					return Command{}, fmt.Errorf("unknown type %q (valid: %s)", t, strings.Join(config.Types, ", "))
				}
				c.Types = append(c.Types, t)
			}
		}
	}
	return c, nil
}

func (c Command) Applies(projectType string) bool {
	return c.Types == nil || slices.Contains(c.Types, projectType)
}

// LoadDir reads every command file in dir; bad files become warnings, never errors.
func LoadDir(dir, kind string) (cmds []Command, warnings []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".example") {
			continue
		}
		path := filepath.Join(dir, name)
		if !ValidName(name) {
			warnings = append(warnings, fmt.Sprintf("%s: %q is not a valid command name (a-z 0-9 -); skipped", shortPath(path), name))
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil {
			var c Command
			if c, err = Parse(name, kind, path, data); err == nil {
				cmds = append(cmds, c)
				continue
			}
		}
		warnings = append(warnings, fmt.Sprintf("%s: %v; skipped", shortPath(path), err))
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	return cmds, warnings
}

// shortPath shows paths under $HOME as ~/… (same rule as cmd/table.go; internal cannot import cmd).
func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return p
}
