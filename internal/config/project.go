package config

import (
	"errors"
	"fmt"
	"os"
	"os/user"
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
	SharedServices  = []string{"mysql", "redis", "opensearch", "mailpit", "phpmyadmin", "rabbitmq", "mariadb", "mongodb", "mongo-express"}
	ReservedDomains = []string{"mailpit.test", "phpmyadmin.test", "rabbitmq.test", "mongo.test"}
	// DefaultOff lists shared services that stay off until enabled (or a project needs them).
	DefaultOff = []string{"rabbitmq", "mariadb", "mongodb", "mongo-express"}
)

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9-]+$`)
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+test$`)
	nodeRe   = regexp.MustCompile(`^(lts|latest|current|[0-9]+(\.[0-9]+){0,2})$`)
	extRe    = regexp.MustCompile(`^[a-z0-9_]+$`)
	pkgRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)
	userRe   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	nonUser  = regexp.MustCompile(`[^a-z0-9_-]`)
)

// currentUsername is the host user; swapped in tests.
var currentUsername = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

type Project struct {
	Name          string   `yaml:"name"`
	Type          string   `yaml:"type"`
	PHP           string   `yaml:"php"`
	Webserver     string   `yaml:"webserver"`
	Docroot       string   `yaml:"docroot"`
	Domains       []string `yaml:"domains,omitempty"`
	Services      []string `yaml:"services"`
	NodeJS        string   `yaml:"nodejs,omitempty"`
	PHPExtensions []string `yaml:"php_extensions,omitempty"`
	Packages      []string `yaml:"packages,omitempty"`
	User          string   `yaml:"user,omitempty"`
	Root          string   `yaml:"-"`
}

// ProjectPath is the project's config file.
func ProjectPath(root string) string { return filepath.Join(root, ".lodev", "config.yaml") }

// ValidationError is a config that parsed but failed Validate.
type ValidationError struct{ Err error }

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

// FindRoot walks up from start to the directory holding .lodev/config.yaml.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(ProjectPath(dir)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not a lodev project (run lodev create)")
		}
		dir = parent
	}
}

func LoadProject(root string) (*Project, error) {
	data, err := os.ReadFile(ProjectPath(root))
	if err != nil {
		return nil, err
	}
	p, err := ParseProject(root, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ProjectPath(root), err)
	}
	return p, nil
}

// ParseProject decodes a project config, applies defaults and validates it.
// A *ValidationError comes with the decoded project; a YAML error with nil.
func ParseProject(root string, data []byte) (*Project, error) {
	p := &Project{}
	if err := yaml.Unmarshal(data, p); err != nil {
		return nil, err
	}
	p.Root = root
	p.ApplyDefaults()
	if err := p.Validate(); err != nil {
		return p, &ValidationError{err}
	}
	return p, nil
}

func SaveProject(p *Project) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ProjectPath(p.Root)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ProjectPath(p.Root), data, 0o644)
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

// CheckName validates a project name.
func CheckName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("name %q is invalid (allowed: a-z, 0-9, -)", name)
	}
	if name == "lodev-services" { // the shared stack's cert and hosts block use this name
		return fmt.Errorf("name %q is reserved by lodev", name)
	}
	return nil
}

func (p *Project) Validate() error {
	if err := CheckName(p.Name); err != nil {
		return err
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
	if d := p.Docroot; d != "" {
		if c := filepath.Clean(d); filepath.IsAbs(d) || c == ".." || strings.HasPrefix(c, "../") {
			return fmt.Errorf("docroot %q must be a folder inside the project", d)
		}
	}
	for _, d := range p.Domains {
		if !domainRe.MatchString(d) {
			return fmt.Errorf("domain %q is invalid (lowercase host name ending in .test)", d)
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
	if p.Uses("mysql") && p.Uses("mariadb") {
		return errors.New("use either mysql or mariadb, not both")
	}
	if p.NodeJS != "" && !nodeRe.MatchString(p.NodeJS) {
		return fmt.Errorf("nodejs %q is invalid (allowed: lts, latest, current, or a version like 20 or 18.19.0)", p.NodeJS)
	}
	for _, e := range p.PHPExtensions {
		if !extRe.MatchString(e) {
			return fmt.Errorf("php_extensions %q is invalid (allowed: a-z, 0-9, _)", e)
		}
	}
	for _, k := range p.Packages {
		if !pkgRe.MatchString(k) {
			return fmt.Errorf("packages %q is invalid (allowed: 2+ chars of a-z, 0-9, + . -, starting with a letter or digit)", k)
		}
	}
	return nil
}

// SanitizeUser maps any string to a valid container user name (a valid one is unchanged).
func SanitizeUser(s string) string {
	s = nonUser.ReplaceAllString(strings.ToLower(s), "_")
	if s != "" && (s[0] == '-' || (s[0] >= '0' && s[0] <= '9')) {
		s = "_" + s
	}
	if len(s) > 32 { // all ASCII by now
		s = s[:32]
	}
	switch s {
	case "":
		return "lodev"
	// root, www-data and the Debian system users present in the image
	case "root", "www-data", "daemon", "bin", "sys", "sync", "games", "man", "lp", "mail", "news",
		"uucp", "proxy", "backup", "list", "irc", "_apt", "nobody", "messagebus",
		"systemd-network", "systemd-timesync":
		return s + "_user"
	}
	return s
}

// ContainerUser is the in-container user: p.User or the host user, sanitized.
// from is the original name when sanitizing changed it, else "".
func (p *Project) ContainerUser() (name, from string) {
	wanted := p.User
	if wanted == "" {
		wanted, _ = currentUsername() // error: "" sanitizes to lodev
	}
	if name = SanitizeUser(wanted); name != wanted {
		from = wanted
	}
	return name, from
}

func (p *Project) DBName() string { return strings.ReplaceAll(p.Name, "-", "_") }

func (p *Project) Uses(svc string) bool { return slices.Contains(p.Services, svc) }

// DB is the shared database the project uses: "mysql", "mariadb", or "" for none.
func (p *Project) DB() string {
	for _, db := range []string{"mariadb", "mysql"} {
		if p.Uses(db) {
			return db
		}
	}
	return ""
}

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
	case exists(filepath.Join(root, "bin", "magento")): // before artisan: Magento trees can carry both
		return "magento"
	case exists(filepath.Join(root, "artisan")):
		return "laravel"
	case exists(filepath.Join(root, "wp-login.php")), exists(filepath.Join(root, "wp-config.php")), exists(filepath.Join(root, "wp-includes")):
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

// ProjectImage is the per-project web image lodev builds.
func ProjectImage(name string) string { return "lodev/" + name + "-web" }

// PHPImage is the image a project on PHP ver runs.
func PHPImage(ver string) string {
	if slices.Contains(BundledPHP, ver) {
		return "lodev/php"
	}
	// legacy- keeps clear of the old per-version lodev/php:<ver> images.
	return "lodev/php:legacy-" + ver
}
