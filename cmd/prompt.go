package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"lodev/internal/config"
	"lodev/internal/dotlodev_assets"
)

// askProject asks, one line each like `ddev config`, for every value whose flag was not given
// (set is keyed by flag name).
func askProject(in io.Reader, out io.Writer, p *config.Project, set map[string]bool) error {
	r := bufio.NewReader(in)
	oldName, oldType := p.Name, p.Type
	steps := []struct {
		flag, title string
		val         *string
		opts        []string
		check       func(string) error
	}{
		{"name", "Project name", &p.Name, nil, config.CheckName},
		{"type", "Project type", &p.Type, config.Types, nil},
		{"php", "PHP version", &p.PHP, config.SupportedPHP, nil},
	}
	for _, s := range steps {
		if !set[s.flag] {
			v, err := choose(in, r, out, s.flag, s.title, *s.val, s.opts, s.check)
			if err != nil {
				return err
			}
			*s.val = v
		}
	}
	settleAnswers(p, oldName, oldType, set["docroot"])
	if !set["docroot"] {
		def := p.Docroot
		if def == "" {
			def = "."
		}
		v, err := ask(r, out, `Document root, relative to the project; "." for the project root`, def, nil, nil)
		if err != nil {
			return err
		}
		p.Docroot = v
	}
	if !set["webserver"] {
		v, err := choose(in, r, out, "webserver", "Webserver", p.Webserver, config.Webservers, nil)
		if err != nil {
			return err
		}
		p.Webserver = v
	}
	settleAnswers(p, p.Name, p.Type, true)
	p.ApplyDefaults()
	return p.Validate()
}

// selectFlags are asked with an arrow-key list on a terminal, by line otherwise.
var selectFlags = []string{"type", "webserver"}

func choose(in io.Reader, r *bufio.Reader, w io.Writer, flag, title, def string, opts []string, check func(string) error) (string, error) {
	if slices.Contains(selectFlags, flag) && useSelect(in) {
		return selectFn(in, w, title, opts, def)
	}
	return ask(r, w, title, def, opts, check)
}

// ask prints "title [opts] (def): " and reads one answer; Enter keeps def, a bad answer asks again.
func ask(r *bufio.Reader, w io.Writer, title, def string, opts []string, check func(string) error) (string, error) {
	prompt := title
	if len(opts) > 0 {
		prompt += " [" + strings.Join(opts, ", ") + "]"
	}
	prompt += " (" + def + "): "
	for {
		fmt.Fprint(w, prompt)
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("init aborted")
		}
		v := strings.TrimSpace(line)
		if v == "" {
			v = def
		}
		var bad error
		if len(opts) > 0 && !slices.Contains(opts, v) {
			bad = fmt.Errorf("must be one of: %s", strings.Join(opts, ", "))
		} else if check != nil {
			bad = check(v)
		}
		if bad == nil {
			return v, nil
		}
		fmt.Fprintf(w, "  %v\n", bad)
	}
}

// settleAnswers makes values derived from earlier defaults follow the user's answers.
func settleAnswers(p *config.Project, oldName, oldType string, docrootSet bool) {
	if p.Name != oldName && slices.Equal(p.Domains, []string{oldName + "." + config.TLD()}) {
		p.Domains = nil
	}
	if p.Type != oldType {
		p.Services = defaultServices[p.Type]
		if !docrootSet {
			p.Docroot = config.DefaultDocroot(p.Type)
		}
	}
	if p.Docroot == "." {
		p.Docroot = ""
	}
}

// scaffold copies dotlodev_assets into .lodev: inert *.example files showing where to customize,
// and php/prepend.php. Existing files are kept, and only the project's webserver folder is copied.
func scaffold(root, webserver string) error {
	// A symlinked homeadditional (a teammate's dotfiles) may not exist here: never write through it.
	fi, err := os.Lstat(filepath.Join(root, ".lodev", "homeadditional"))
	symlinkedHome := err == nil && fi.Mode()&os.ModeSymlink != 0
	return fs.WalkDir(dotlodev_assets.FS, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch top, _, _ := strings.Cut(rel, "/"); {
		case (top == "nginx" || top == "apache") && top != webserver, top == "homeadditional" && symlinkedHome:
			return nil
		}
		path := filepath.Join(root, ".lodev", filepath.FromSlash(rel))
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		data, err := dotlodev_assets.FS.ReadFile(rel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	})
}
