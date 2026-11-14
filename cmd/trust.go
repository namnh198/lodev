package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"lodev/internal/config"
	"lodev/internal/docker"
)

// trustItem is one command the project's repo makes lodev run: a stable Key and the text shown
// when asking.
type trustItem struct{ Key, Display string }

// trustItems lists everything in p's repo that lodev will run, sorted by Key.
func trustItems(p *config.Project) ([]trustItem, error) {
	byKey := map[string]trustItem{}
	add := func(key, display string) {
		if _, ok := byKey[key]; !ok { // the same script under two events is one item
			byKey[key] = trustItem{key, display}
		}
	}
	// addFile lists a repo file; a missing one is listed too, since the hook fails later on its own.
	addFile := func(rel, display string) error {
		_, err := os.Stat(filepath.Join(p.Root, rel))
		if errors.Is(err, fs.ErrNotExist) {
			display += " (missing)"
		} else if err != nil {
			return err
		}
		add(rel, display)
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
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() { // a file named *.d is no hook folder
			continue
		}
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
				add(fmt.Sprintf("hooks.%s[%d]", ev, i), display)
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
		add("cron", "cron: "+strings.Join(lines, "\ncron: "))
	}
	for _, d := range p.WebExtraDaemons {
		dir := d.Directory
		if dir == "" {
			dir = p.WorkDir()
		}
		add("web_extra_daemons."+d.Name, fmt.Sprintf("daemon %s: %s (in %s)", d.Name, d.Command, dir))
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
		add(part.Name, "web image: "+part.Name)
	}

	// addTree lists the files and symlinks below the repo folder dir.
	addTree := func(dir, label string, skipExamples bool) error {
		return walkTree(filepath.Join(p.Root, dir), func(_, rel string, d fs.DirEntry) error {
			if skipExamples && strings.HasSuffix(rel, ".example") || !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
				return nil
			}
			key := dir + "/" + rel
			add(key, label+key)
			return nil
		})
	}
	if err := addTree(".lodev/homeadditional", "home file: ", true); err != nil {
		return nil, err
	}
	// Apache runs piped logs and other commands from its config; nginx's config cannot.
	if err := addTree(".lodev/apache", "apache config: ", true); err != nil {
		return nil, err
	}
	if conf := ".lodev/apache-webserver.conf"; !generated(filepath.Join(p.Root, conf)) {
		if err := addFile(conf, "apache config: "+conf); err != nil {
			return nil, err
		}
	}
	if len(p.Environment) > 0 {
		var lines []string
		for _, k := range slices.Sorted(maps.Keys(p.Environment)) {
			lines = append(lines, k+"="+p.Environment[k])
		}
		add("environment", "environment: "+strings.Join(lines, "\nenvironment: "))
	}

	items := make([]trustItem, 0, len(byKey))
	for _, it := range byKey {
		items = append(items, it)
	}
	slices.SortFunc(items, func(a, b trustItem) int { return strings.Compare(a.Key, b.Key) })
	return items, nil
}

// walkTree calls fn for every entry below dir with its slash path relative to dir, following dir
// itself when it is a symlink (start mounts what it points to). A missing dir has no entries.
func walkTree(dir string, fn func(path, rel string, d fs.DirEntry) error) error {
	root, err := filepath.EvalSymlinks(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return fn(path, filepath.ToSlash(rel), d)
	})
}

// trustTTY reports whether lodev can ask; swapped in tests.
var trustTTY = stdinTTY

// confirmRepoCommands lists what p's repo makes lodev run and asks before lodev create registers
// it; yes approves. Nothing is stored: start, restart and hooks do not ask again.
func confirmRepoCommands(p *config.Project, yes bool) error {
	items, err := trustItems(p)
	if err != nil || len(items) == 0 || yes {
		return err
	}
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = "  " + strings.ReplaceAll(it.Display, "\n", "\n  ")
	}
	list := fmt.Sprintf("project %s runs commands from its repo:\n%s", p.Name, strings.Join(lines, "\n"))
	if !trustTTY() {
		return fmt.Errorf("%s\nrun lodev create in a terminal to review them, or pass --yes", list)
	}
	fmt.Println(list)
	if !confirm("Run these commands?", false) {
		return errors.New("not approved: nothing was registered")
	}
	return nil
}
