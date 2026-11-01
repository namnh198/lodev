package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	"charm.land/huh/v2"

	"lodev/internal/config"
)

// askProject prompts for every value whose flag was not given (set is keyed by flag name).
func askProject(p *config.Project, set map[string]bool) error {
	oldName, oldType := p.Name, p.Type
	var f1 []huh.Field
	if !set["name"] {
		f1 = append(f1, huh.NewInput().Title("Project name").Value(&p.Name).Validate(config.CheckName))
	}
	if !set["type"] {
		f1 = append(f1, huh.NewSelect[string]().Title("Project type").Options(huh.NewOptions(config.Types...)...).Value(&p.Type))
	}
	if !set["php"] {
		f1 = append(f1, huh.NewSelect[string]().Title("PHP version").Options(huh.NewOptions(config.SupportedPHP...)...).Value(&p.PHP))
	}
	if err := runFields(f1); err != nil {
		return err
	}
	settleAnswers(p, oldName, oldType, set["docroot"])
	var f2 []huh.Field
	if !set["docroot"] {
		f2 = append(f2, huh.NewInput().Title("Document root").Description("relative to the project; empty for the project root").
			Placeholder("(project root)").Value(&p.Docroot))
	}
	if !set["webserver"] {
		f2 = append(f2, huh.NewSelect[string]().Title("Webserver").Options(huh.NewOptions(config.Webservers...)...).Value(&p.Webserver))
	}
	if err := runFields(f2); err != nil {
		return err
	}
	settleAnswers(p, p.Name, p.Type, true)
	p.ApplyDefaults()
	return p.Validate()
}

// settleAnswers makes values derived from earlier defaults follow the user's answers.
func settleAnswers(p *config.Project, oldName, oldType string, docrootSet bool) {
	if p.Name != oldName && slices.Equal(p.Domains, []string{oldName + ".test"}) {
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

func runFields(fields []huh.Field) error {
	if len(fields) == 0 {
		return nil
	}
	if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("init aborted")
		}
		return err
	}
	return nil
}

// scaffold writes inert *.example files showing where to customize; existing files are kept.
func scaffold(root, webserver string) error {
	files := map[string]string{
		"web-images/Dockerfile.example": "# Rename to Dockerfile to extend the web image.\nRUN apt-get update && apt-get install -y --no-install-recommends imagemagick\n",
		"php/example.ini.example":       "; Rename to <name>.ini to apply PHP settings.\n; memory_limit = 4G\n",
		"example.compose.yml.example":   "# Rename to <name>.compose.yml to merge extra services.\n# services:\n#   web:\n#     networks: [lodev, default]\n",
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
