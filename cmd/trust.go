package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"lodev/internal/commands"
	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/dotenv"
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
		add("web_extra_daemons."+d.Name, fmt.Sprintf("daemon %s: %s (in %s)", d.Name, d.Command, p.DaemonDir(d)))
	}
	if p.Type == "nextjs" && userNextjsScript(p) {
		add(config.NextjsDaemon, "nextjs: .lodev/daemons/nextjs.sh")
	} else if p.Type == "nextjs" {
		pm, prod, scripts := p.NodePackageManager(), p.NextjsProdScript(), p.NodeScripts()
		var shown []string
		for _, name := range []string{"dev", "build", prod} {
			v := "none"
			if s, ok := scripts[name]; ok {
				v = fmt.Sprintf("%q", s)
			}
			shown = append(shown, name+": "+v)
		}
		add(config.NextjsDaemon, fmt.Sprintf("nextjs: %s && %s run dev, or with NODE_ENV=production %s run build && %s run %s (package.json %s)",
			p.NodeInstallCommand(), pm, pm, pm, prod, strings.Join(shown, ", ")))
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
	addTree := func(dir, label string) error {
		return walkTree(filepath.Join(p.Root, dir), func(_, rel string, d fs.DirEntry) error {
			if d.IsDir() {
				return nil
			}
			key := dir + "/" + rel
			add(key, label+key)
			return nil
		})
	}
	shipped := map[string]bool{}
	for _, c := range shippedCommands() {
		shipped[c.Name] = true
	}
	for kind, where := range map[string]string{commands.Web: "web container", commands.Host: "runs on this machine"} {
		cmds, _ := commands.LoadDir(filepath.Join(p.Root, ".lodev", "commands", kind), kind)
		for _, c := range cmds {
			rel := ".lodev/commands/" + kind + "/" + c.Name
			display := fmt.Sprintf("command lodev %s (%s): %s", c.Name, where, rel)
			if shipped[c.Name] {
				display += ", replaces lodev's " + c.Name
			}
			add(rel, display)
			if c.Complete != nil {
				rel := ".lodev/commands/" + kind + "/autocomplete/" + c.Name
				add(rel, fmt.Sprintf("autocomplete for lodev %s (%s): %s", c.Name, where, rel))
			}
		}
	}
	if err := addTree(".lodev/homeadditional", "home file: "); err != nil {
		return nil, err
	}
	// Apache runs piped logs and other commands from its config; nginx's config cannot.
	if err := addTree(".lodev/apache", "apache config: "); err != nil {
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
	// Names only: .lodev/.env is meant for secrets, and the question is printed.
	if data, err := os.ReadFile(dotenvPath(p)); err == nil {
		entries, err := dotenv.Parse(data)
		if err != nil {
			return nil, fmt.Errorf(".lodev/.env: %w", err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Key)
		}
		slices.Sort(names)
		add(".lodev/.env", "environment (.lodev/.env): "+strings.Join(slices.Compact(names), ", "))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	items := make([]trustItem, 0, len(byKey))
	for _, it := range byKey {
		items = append(items, it)
	}
	slices.SortFunc(items, func(a, b trustItem) int { return strings.Compare(a.Key, b.Key) })
	return items, nil
}

// walkTree calls fn for the folders, regular files and symlinks below the repo folder dir, with their
// slash path relative to dir, following dir itself when it is a symlink; *.example files are left
// out. It is the one place that decides which repo files the trust list shows and start copies. A
// missing dir has no entries.
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
		rel = filepath.ToSlash(rel)
		if !d.IsDir() && (strings.HasSuffix(rel, ".example") || !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0) {
			return nil
		}
		return fn(path, rel, d)
	})
}

// escapeControl writes the terminal controls in s (but newline and tab) as Go escapes, so repo text
// cannot move the cursor, erase or reorder the lines it is approved from.
func escapeControl(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		switch {
		case r == utf8.RuneError && n == 1: // a lone byte such as 0x9b, the 8-bit CSI
			fmt.Fprintf(&b, `\x%02x`, s[0])
		case r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)):
			b.WriteString(strings.Trim(strconv.QuoteRune(r), "'"))
		default:
			b.WriteString(s[:n])
		}
		s = s[n:]
	}
	return b.String()
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
		lines[i] = "  " + strings.ReplaceAll(escapeControl(it.Display), "\n", "\n  ")
	}
	list := fmt.Sprintf("project %s runs commands from its repo:\n%s", p.Name, strings.Join(lines, "\n"))
	if !trustTTY() {
		return fmt.Errorf("%s\nrun lodev create in a terminal to review them, or pass --yes", list)
	}
	fmt.Println(list)
	if !confirmDefaultYes("Run these commands?") {
		return errors.New("not approved: nothing was registered")
	}
	return nil
}
