// Package registry tracks known projects in ~/.lodev/projects.json.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"lodev/internal/config"
)

type Entry struct {
	Path    string   `json:"path"`
	Domains []string `json:"domains"`
}

type Registry map[string]Entry

func file() string { return filepath.Join(config.Home(), "projects.json") }

func Load() (Registry, error) {
	r := Registry{}
	data, err := os.ReadFile(file())
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	return r, json.Unmarshal(data, &r)
}

func (r Registry) Save() error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.Home(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file(), data, 0o644)
}

// stale entries point at a folder that is no longer a lodev project (moved or deleted).
func stale(e Entry) bool {
	_, err := os.Stat(filepath.Join(e.Path, ".lodev", "config.yaml"))
	return err != nil
}

func (r Registry) Conflict(p *config.Project) error {
	for _, name := range r.Names() {
		e := r[name]
		if e.Path == p.Root || stale(e) {
			continue
		}
		if name == p.Name {
			return fmt.Errorf("project name %q is already used by %s", name, e.Path)
		}
		for _, d := range p.WebDomains() {
			if slices.Contains(e.Domains, d) {
				return fmt.Errorf("domain %s is already used by project %q (%s)", d, name, e.Path)
			}
		}
	}
	return nil
}

// Put records p and returns the old names dropped for the same folder (a renamed `name:`).
func (r Registry) Put(p *config.Project) []string {
	var dropped []string
	for name, e := range r {
		if e.Path == p.Root && name != p.Name {
			delete(r, name)
			dropped = append(dropped, name)
		}
	}
	r[p.Name] = Entry{Path: p.Root, Domains: p.WebDomains()}
	return dropped
}

func (r Registry) Names() []string {
	names := make([]string, 0, len(r))
	for n := range r {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (r Registry) AllDomains() []string {
	var all []string
	for _, e := range r {
		all = append(all, e.Domains...)
	}
	sort.Strings(all)
	return slices.Compact(all)
}
