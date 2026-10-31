package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	SupportedPHP    = []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}
	BundledPHP      = []string{"8.2", "8.3", "8.4", "8.5"} // in the base image; others get a legacy image
	Types           = []string{"laravel", "wordpress", "magento", "php"}
	Webservers      = []string{"nginx", "apache", "none"}
	SharedServices  = []string{"mysql", "redis", "opensearch", "mailpit", "phpmyadmin"}
	ReservedDomains = []string{"mailpit.test", "phpmyadmin.test"}
)

var nameRe = regexp.MustCompile(`^[a-z0-9-]+$`)

type Project struct {
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"`
	PHP       string   `yaml:"php"`
	Webserver string   `yaml:"webserver"`
	Docroot   string   `yaml:"docroot"`
	Domains   []string `yaml:"domains,omitempty"`
	Services  []string `yaml:"services"`
	Root      string   `yaml:"-"`
}

func configPath(root string) string { return filepath.Join(root, ".lodev", "config.yaml") }

// FindRoot walks up from start to the directory holding .lodev/config.yaml.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(configPath(dir)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not a lodev project (run lodev init)")
		}
		dir = parent
	}
}

func LoadProject(root string) (*Project, error) {
	data, err := os.ReadFile(configPath(root))
	if err != nil {
		return nil, err
	}
	p := &Project{}
	if err := yaml.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("%s: %w", configPath(root), err)
	}
	p.Root = root
	p.ApplyDefaults()
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", configPath(root), err)
	}
	return p, nil
}

func SaveProject(p *Project) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath(p.Root)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(configPath(p.Root), data, 0o644)
}

func (p *Project) ApplyDefaults() {
	if p.Webserver == "" {
		p.Webserver = "nginx"
	}
	if len(p.Domains) == 0 {
		p.Domains = []string{p.Name + ".test"}
	}
}

func oneOf(field, v string, allowed []string) error {
	if slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%s %q is invalid (allowed: %s)", field, v, strings.Join(allowed, ", "))
}

func (p *Project) Validate() error {
	if !nameRe.MatchString(p.Name) {
		return fmt.Errorf("name %q is invalid (allowed: a-z, 0-9, -)", p.Name)
	}
	if err := oneOf("type", p.Type, Types); err != nil {
		return err
	}
	if err := oneOf("php", p.PHP, SupportedPHP); err != nil {
		return err
	}
	if err := oneOf("webserver", p.Webserver, Webservers); err != nil {
		return err
	}
	for _, d := range p.Domains {
		if !strings.HasSuffix(d, ".test") {
			return fmt.Errorf("domain %q must end with .test", d)
		}
		if slices.Contains(ReservedDomains, d) {
			return fmt.Errorf("domain %q is reserved by lodev", d)
		}
	}
	for _, s := range p.Services {
		if err := oneOf("service", s, SharedServices); err != nil {
			return err
		}
	}
	return nil
}

func (p *Project) DBName() string { return strings.ReplaceAll(p.Name, "-", "_") }

func (p *Project) Uses(svc string) bool { return slices.Contains(p.Services, svc) }

// WebDomains is the routed domains; CLI-only projects have none.
func (p *Project) WebDomains() []string {
	if p.Webserver == "none" {
		return nil
	}
	return p.Domains
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func DetectType(root string) string {
	switch {
	case exists(filepath.Join(root, "artisan")):
		return "laravel"
	case exists(filepath.Join(root, "bin", "magento")):
		return "magento"
	case exists(filepath.Join(root, "wp-config.php")), exists(filepath.Join(root, "wp-includes")):
		return "wordpress"
	}
	return "php"
}

func DefaultDocroot(typ string) string {
	switch typ {
	case "laravel":
		return "public"
	case "magento":
		return "pub"
	}
	return ""
}

var nonName = regexp.MustCompile(`[^a-z0-9]+`)

// SanitizeName turns a folder name into a valid project name.
func SanitizeName(dir string) string {
	return strings.Trim(nonName.ReplaceAllString(strings.ToLower(dir), "-"), "-")
}

// PHPImage is the image a project on PHP ver runs.
func PHPImage(ver string) string {
	if slices.Contains(BundledPHP, ver) {
		return "lodev/php"
	}
	// legacy- keeps clear of the old per-version lodev/php:<ver> images.
	return "lodev/php:legacy-" + ver
}
