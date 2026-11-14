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
	// Trusted is the approved repo commands. nil: registered before approvals existed; empty: approved, nothing to run.
	Trusted map[string]string `json:"trusted"`
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
	// A new file renamed into place: a crash or full disk never leaves broken JSON behind.
	f, err := os.CreateTemp(config.Home(), "projects.json.*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o644)
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), file())
}

// SamePath reports whether a and b are one folder, also when one reaches it through a symlink
// (os.Getwd keeps the symlinked path the shell was in). A path that cannot be resolved compares as given.
func SamePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	return err == nil && ra == rb
}

// stale entries point at a folder that is no longer a lodev project (moved or deleted).
func stale(e Entry) bool {
	_, err := os.Stat(filepath.Join(e.Path, ".lodev", "config.yaml"))
	return err != nil
}

// Conflict refuses p's name, a web domain or an extra host (its services' VIRTUAL_HOST) that another project uses.
func (r Registry) Conflict(p *config.Project, extra ...string) error {
	for _, name := range r.Names() {
		e := r[name]
		if SamePath(e.Path, p.Root) || stale(e) {
			continue
		}
		if name == p.Name {
			return fmt.Errorf("project name %q is already used by %s", name, e.Path)
		}
		for _, d := range slices.Concat(p.WebDomains(), extra) {
			if slices.Contains(e.Domains, d) {
				return fmt.Errorf("domain %s is already used by project %q (%s)", d, name, e.Path)
			}
		}
	}
	return nil
}

// Put records p with its web domains and extra hosts, and returns the old names dropped for the
// same folder (a renamed `name:`).
func (r Registry) Put(p *config.Project, extra ...string) []string {
	// Approvals follow the folder: the entry under the same name first, else the one this project is renamed from.
	var dropped []string
	var trusted map[string]string
	kept := false
	if e, ok := r[p.Name]; ok && SamePath(e.Path, p.Root) {
		trusted, kept = e.Trusted, true
	}
	for name, e := range r {
		if SamePath(e.Path, p.Root) && name != p.Name {
			if !kept {
				trusted, kept = e.Trusted, true
			}
			delete(r, name)
			dropped = append(dropped, name)
		}
	}
	r[p.Name] = Entry{Path: p.Root, Domains: slices.Concat(p.WebDomains(), extra), Trusted: trusted}
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
