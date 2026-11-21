// Package commands parses and loads custom command files (script files that become `lodev <name>`).
package commands

import (
	"fmt"
	"maps"
	"os"
	"path"
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
	Marker     bool     // see HasMarker
	Interp     bool     // line 1 is a non-bash shebang: run the file with that interpreter
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func ValidName(name string) bool { return nameRe.MatchString(name) }

// Parse reads the header (the leading run of `#` lines); only an unknown Type is an error.
func Parse(name, kind, path string, data []byte) (Command, error) {
	script := strings.ReplaceAll(string(data), "\r\n", "\n")
	c := Command{Name: name, Kind: kind, Path: path, Script: script}
	c.Marker, c.Interp = HasMarker([]byte(script)), isInterp(script)
	c.Use = name + " [args...]"
	c.Short = "Custom command (" + ShortPath(path) + ")"
	for _, line := range strings.Split(script, "\n") {
		if !strings.HasPrefix(line, "#") {
			break
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

// HasMarker reports whether templates.GeneratedMarker is line 1, or line 2 after a `#!` line.
func HasMarker(data []byte) bool {
	first, rest, _ := strings.Cut(string(data), "\n")
	if strings.HasPrefix(first, "#!") {
		first, _, _ = strings.Cut(rest, "\n")
	}
	return first == templates.GeneratedMarker
}

// isInterp reports whether line 1 is a shebang for something other than bash (`#!/bin/bash`
// or `#!/usr/bin/env bash`).
func isInterp(script string) bool {
	line, _, _ := strings.Cut(script, "\n")
	f := strings.Fields(strings.TrimPrefix(line, "#!"))
	if !strings.HasPrefix(line, "#!") || len(f) == 0 {
		return false
	}
	prog := path.Base(f[0])
	if prog == "env" && len(f) > 1 {
		prog = path.Base(f[1])
	}
	return prog != "bash"
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
			warnings = append(warnings, fmt.Sprintf("%s: %q is not a valid command name (a-z 0-9 -); skipped", ShortPath(path), name))
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
		warnings = append(warnings, fmt.Sprintf("%s: %v; skipped", ShortPath(path), err))
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	return cmds, warnings
}

// ShortPath shows paths under $HOME as ~/….
func ShortPath(p string) string {
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

// loadScope reads dir/web and dir/host; a name in both keeps the web one.
func loadScope(dir string, keep func(Command) bool) (map[string]Command, []string) {
	web, warnings := LoadDir(filepath.Join(dir, Web), Web)
	host, w := LoadDir(filepath.Join(dir, Host), Host)
	warnings = append(warnings, w...)
	byName := map[string]Command{}
	for _, c := range append(web, host...) {
		if !keep(c) {
			continue
		}
		if prev, ok := byName[c.Name]; ok {
			warnings = append(warnings, fmt.Sprintf("%s: %q is also in %s; using the web one", ShortPath(c.Path), c.Name, ShortPath(prev.Path)))
			continue
		}
		byName[c.Name] = c
	}
	return byName, warnings
}

// Resolve layers shipped < global (<home>/commands, marker files dropped) < project
// (<projectRoot>/.lodev/commands, only when registered), keeps the commands that apply to
// projectType (all when it is ""), and skips builtin names. projectRoot "" means outside a project.
func Resolve(shipped []Command, home string, projectRoot, projectType string, registered bool, builtin map[string]bool) (cmds []Command, warnings []string) {
	applies := func(c Command) bool { return projectType == "" || c.Applies(projectType) }

	byName := map[string]Command{}
	for _, c := range shipped {
		if applies(c) {
			byName[c.Name] = c
		}
	}
	// A marker file in ~/.lodev/commands is lodev's synced copy of a shipped one, not the user's.
	global, warnings := loadScope(filepath.Join(home, "commands"), func(c Command) bool { return !c.Marker && applies(c) })
	for n, c := range global {
		byName[n] = c
	}
	if projectRoot != "" {
		dir := filepath.Join(projectRoot, ".lodev", "commands")
		if registered {
			project, w := loadScope(dir, applies)
			warnings = append(warnings, w...)
			for n, c := range project {
				byName[n] = c
			}
		} else {
			web, _ := LoadDir(filepath.Join(dir, Web), Web)
			host, _ := LoadDir(filepath.Join(dir, Host), Host)
			if len(web)+len(host) > 0 {
				warnings = append(warnings, ".lodev/commands is not loaded: run lodev create to review the project's commands")
			}
		}
	}

	for _, n := range slices.Sorted(maps.Keys(byName)) {
		c := byName[n]
		if builtin[n] {
			warnings = append(warnings, fmt.Sprintf("%s: %q is a lodev command; skipped", ShortPath(c.Path), n))
			continue
		}
		cmds = append(cmds, c)
	}
	return cmds, warnings
}

// HostEnvKeys is every variable HostEnv can set; inherited copies are dropped so a nested
// command never sees another project's values.
var HostEnvKeys = []string{"LODEV_BIN", "LODEV_PWD", "LODEV_PROJECT", "LODEV_ROOT", "LODEV_TYPE", "LODEV_URL",
	"LODEV_WEB_CONTAINER", "LODEV_CONTAINER_USER", "LODEV_WORKDIR", "LODEV_PROJECT_STATUS",
	"LODEV_DB_TYPE", "LODEV_DB_HOST", "LODEV_DB_PORT", "LODEV_DB_USER", "LODEV_DB_PASSWORD", "LODEV_DB_NAME"}

// DropEnv returns environ without the HostEnvKeys entries.
func DropEnv(environ []string) []string {
	return slices.DeleteFunc(environ, func(e string) bool {
		k, _, _ := strings.Cut(e, "=")
		return slices.Contains(HostEnvKeys, k)
	})
}

// HostEnv is the LODEV_* environment a host command gets on top of the user's.
func HostEnv(p *config.Project, g *config.Global, pwd, bin string, running bool) []string {
	env := []string{"LODEV_BIN=" + bin, "LODEV_PWD=" + pwd}
	if p == nil {
		return env
	}
	url := ""
	if p.Webserver != "none" && len(p.Domains) > 0 {
		url = "https://" + p.Domains[0]
	}
	user, _ := p.ContainerUser()
	status := "stopped"
	if running {
		status = "running"
	}
	env = append(env,
		"LODEV_PROJECT="+p.Name,
		"LODEV_ROOT="+p.Root,
		"LODEV_TYPE="+p.Type,
		"LODEV_URL="+url,
		"LODEV_WEB_CONTAINER="+p.WebContainer(),
		"LODEV_CONTAINER_USER="+user,
		"LODEV_WORKDIR="+p.WorkDir(),
		"LODEV_PROJECT_STATUS="+status,
	)
	if db := p.DB(); db != "" {
		port := g.Ports.MySQL
		if db == "mariadb" {
			port = g.Ports.MariaDB
		}
		env = append(env,
			"LODEV_DB_TYPE="+db,
			"LODEV_DB_HOST=127.0.0.1",
			"LODEV_DB_PORT="+port,
			"LODEV_DB_USER=root",
			"LODEV_DB_PASSWORD=root",
			"LODEV_DB_NAME="+p.DBName(),
		)
	}
	return env
}
