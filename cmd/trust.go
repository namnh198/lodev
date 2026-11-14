package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/registry"
)

// trustItem is one command the project's repo makes lodev run: a stable Key, the text shown
// when asking, and the sha256 of its content.
type trustItem struct{ Key, Display, Sum string }

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// trustItems lists everything in p's repo that lodev will run, sorted by Key.
func trustItems(p *config.Project) ([]trustItem, error) {
	byKey := map[string]trustItem{}
	add := func(key, display string, content []byte) {
		if _, ok := byKey[key]; !ok { // the same script under two events is one item
			byKey[key] = trustItem{key, display, sha(content)}
		}
	}
	// addFile hashes a repo file; a missing one is listed, since the hook fails later on its own.
	addFile := func(rel, display string) error {
		b, err := os.ReadFile(filepath.Join(p.Root, rel))
		if errors.Is(err, fs.ErrNotExist) {
			display += " (missing)"
		} else if err != nil {
			return err
		}
		add(rel, display, b)
		return nil
	}

	events := make([]string, 0, len(p.Hooks))
	for ev := range p.Hooks {
		events = append(events, ev)
	}
	dirs, err := filepath.Glob(filepath.Join(p.Root, ".lodev", "hooks", "*.d"))
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if ev := strings.TrimSuffix(filepath.Base(d), ".d"); !slices.Contains(events, ev) {
			events = append(events, ev)
		}
	}
	for _, ev := range events {
		steps, err := hookSteps(p, ev)
		if err != nil {
			return nil, err
		}
		for i, s := range steps {
			display := fmt.Sprintf("hook %s: %s", ev, s.label)
			if i < len(p.Hooks[ev]) { // config step; the rest are .d files
				add(fmt.Sprintf("hooks.%s[%d]", ev, i), display, []byte(s.label))
				if s.script == "" {
					continue
				}
			}
			if err := addFile(s.script, display); err != nil {
				return nil, err
			}
		}
	}

	if lines := p.CronLines(); len(lines) > 0 {
		add("cron", "cron: "+strings.Join(lines, "\ncron: "), []byte(strings.Join(lines, "\n")))
	}
	for _, d := range p.WebExtraDaemons {
		dir := d.Directory
		if dir == "" {
			dir = p.WorkDir()
		}
		add("web_extra_daemons."+d.Name, fmt.Sprintf("daemon %s: %s (in %s)", d.Name, d.Command, dir),
			[]byte(d.Name+"\n"+dir+"\n"+d.Command))
	}

	composes, err := extraComposeFiles(filepath.Join(p.Root, ".lodev"))
	if err != nil {
		return nil, err
	}
	for _, f := range composes {
		rel := ".lodev/" + filepath.Base(f)
		if err := addFile(rel, "compose: "+rel); err != nil {
			return nil, err
		}
	}
	parts, err := docker.WebImageParts(p.Root)
	if err != nil {
		return nil, err
	}
	for _, part := range parts {
		add(part.Name, "web image: "+part.Name, part.Content)
	}

	home := filepath.Join(p.Root, ".lodev", "homeadditional")
	err = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := ".lodev/homeadditional/" + filepath.ToSlash(strings.TrimPrefix(path, home+string(filepath.Separator)))
		var b []byte
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(path)
			if err != nil {
				return err
			}
			b = []byte(t)
		case d.Type().IsRegular():
			if b, err = os.ReadFile(path); err != nil {
				return err
			}
		default:
			return nil
		}
		add(rel, "home file: "+rel, b)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	items := make([]trustItem, 0, len(byKey))
	for _, it := range byKey {
		items = append(items, it)
	}
	slices.SortFunc(items, func(a, b trustItem) int { return strings.Compare(a.Key, b.Key) })
	return items, nil
}

// trustSums is the key to sum map the registry stores; never nil, so "approved nothing" is kept.
func trustSums(items []trustItem) map[string]string {
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[it.Key] = it.Sum
	}
	return m
}

// trustReport is what differs between the approved set and the repo's current commands.
type trustReport struct {
	Added, Changed []trustItem
	Removed        []string
}

func diffTrust(stored map[string]string, items []trustItem) trustReport {
	var r trustReport
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Key] = true
		if sum, ok := stored[it.Key]; !ok {
			r.Added = append(r.Added, it)
		} else if sum != it.Sum {
			r.Changed = append(r.Changed, it)
		}
	}
	for k := range stored {
		if !seen[k] {
			r.Removed = append(r.Removed, k)
		}
	}
	slices.Sort(r.Removed)
	return r
}

func (r trustReport) Empty() bool {
	return len(r.Added) == 0 && len(r.Changed) == 0 && len(r.Removed) == 0
}

// String lists config items by their command text and files by path plus what happened to them.
func (r trustReport) String() string {
	var b strings.Builder
	section := func(title, fileSuffix string, items []trustItem) {
		if len(items) == 0 {
			return
		}
		b.WriteString(title + ":\n")
		for _, it := range items {
			text := strings.ReplaceAll(it.Display, "\n", "\n  ")
			if !isConfigKey(it.Key) {
				text += fileSuffix
			}
			b.WriteString("  " + text + "\n")
		}
	}
	section("added", " (new)", r.Added)
	section("changed", " (changed — review with git diff)", r.Changed)
	if len(r.Removed) > 0 {
		b.WriteString("removed:\n")
		for _, k := range r.Removed {
			b.WriteString("  " + k + "\n")
		}
	}
	return b.String()
}

func isConfigKey(k string) bool {
	return k == "cron" || strings.HasPrefix(k, "hooks.") || strings.HasPrefix(k, "web_extra_daemons.")
}

// trustTTY reports whether lodev can ask; swapped in tests.
var trustTTY = stdinTTY

// ensureTrusted is the gate before lodev runs anything from p's repo: it returns nil when p's
// commands are the approved ones, records them when yes or the user approves, and errors otherwise.
// locked says the caller holds the registry lock.
func ensureTrusted(p *config.Project, yes, locked bool) error {
	items, err := trustItems(p)
	if err != nil {
		return err
	}
	sums := trustSums(items)
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	e, known := reg[p.Name]
	known = known && registry.SamePath(e.Path, p.Root)
	if known && e.Trusted == nil { // registered by a lodev older than approvals
		return recordTrust(p, sums, locked)
	}
	var stored map[string]string
	if known {
		stored = e.Trusted
	}
	report := diffTrust(stored, items)
	if report.Empty() {
		if known {
			return nil
		}
		return recordTrust(p, sums, locked)
	}
	switch {
	case yes:
	case !trustTTY():
		return fmt.Errorf("project %s runs commands from its repo that you have not approved:\n%s\n"+
			"run lodev start in a terminal to review them, or pass --yes", p.Name, strings.TrimSuffix(report.String(), "\n"))
	default:
		fmt.Printf("project %s runs commands from its repo:\n%s", p.Name, report)
		if !confirm("Run these commands?", false) {
			return errors.New("not approved: nothing was started")
		}
	}
	return recordTrust(p, sums, locked)
}

// recordTrust stores sums as p's approved set, registering p if the registry lacks it.
func recordTrust(p *config.Project, sums map[string]string, locked bool) error {
	save := func() error {
		reg, err := registry.Load()
		if err != nil {
			return err
		}
		if e, ok := reg[p.Name]; !ok || !registry.SamePath(e.Path, p.Root) {
			if err := reg.Conflict(p); err != nil {
				return err
			}
			reg.Put(p)
		}
		e := reg[p.Name]
		e.Trusted = sums
		reg[p.Name] = e
		return reg.Save()
	}
	if locked {
		return save()
	}
	return withLock(save)
}
