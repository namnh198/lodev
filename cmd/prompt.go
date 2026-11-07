package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"lodev/internal/config"
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
			v, err := ask(r, out, s.title, *s.val, s.opts, s.check)
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
		v, err := ask(r, out, "Webserver", p.Webserver, config.Webservers, nil)
		if err != nil {
			return err
		}
		p.Webserver = v
	}
	settleAnswers(p, p.Name, p.Type, true)
	p.ApplyDefaults()
	return p.Validate()
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

// scaffold writes inert *.example files showing where to customize, and php/prepend.php; existing files are kept.
func scaffold(root, webserver string) error {
	files := map[string]string{
		"web-images/Dockerfile.example": "# Rename to Dockerfile to extend the web image.\nRUN apt-get update && apt-get install -y --no-install-recommends imagemagick\n",
		"php/example.ini.example":       "; Rename to <name>.ini to apply PHP settings.\n; memory_limit = 4G\n",
		// zz-lodev.ini sets it as auto_prepend_file: it must exist or every PHP script fails.
		"php/prepend.php":             "<?php\n// Runs before every PHP script (web and CLI). e.g. $_SERVER['MAGE_RUN_CODE'] = 'default';\n",
		"example.compose.yml.example": "# Rename to <name>.compose.yml to merge extra services.\n# services:\n#   adminer:\n#     image: adminer\n#     networks: [lodev, default]   # lodev: reach mysql/redis; default: talk to web\n",
	}
	if webserver == "nginx" || webserver == "apache" {
		files[webserver+"/example.conf.example"] = "# Rename to <name>.conf to extend the " + webserver + " config.\n# location /api { ... }\n"
	}
	for rel, body := range files {
		path := filepath.Join(root, ".lodev", rel)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}
