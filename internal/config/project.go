package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	lodevupdate "lodev/internal/update" // not "update": edit_test.go declares that name
)

var (
	SupportedPHP   = []string{"7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}
	BundledPHP     = []string{"8.2", "8.3", "8.4", "8.5"} // in the base image; others get a legacy image
	Types          = []string{"laravel", "wordpress", "magento", "php"}
	Webservers     = []string{"nginx", "apache", "none"}
	SharedServices = []string{"mysql", "redis", "opensearch", "mailpit", "phpmyadmin", "rabbitmq", "mariadb", "mongodb", "mongo-express"}
	// DefaultOff lists shared services that stay off until enabled (or a project needs them).
	DefaultOff = []string{"rabbitmq", "mariadb", "mongodb", "mongo-express"}
)

var (
	nameRe  = regexp.MustCompile(`^[a-z0-9-]+$`)
	nodeRe  = regexp.MustCompile(`^(lts|latest|current|[0-9]+(\.[0-9]+){0,2})$`)
	extRe   = regexp.MustCompile(`^[a-z0-9_]+$`)
	pkgRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)
	userRe  = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	nonUser = regexp.MustCompile(`[^a-z0-9_-]`)
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
	// WorkingDir is where the project is mounted in the web container (default /app).
	WorkingDir string `yaml:"working_dir,omitempty"`
	// PHPIDEConfig is the PHP_IDE_CONFIG serverName PhpStorm maps (default: the project name).
	PHPIDEConfig string `yaml:"php_ide_config,omitempty"`
	// EnableXdebug turns Xdebug on whenever the project starts; lodev xdebug on/off keeps it in step.
	EnableXdebug bool `yaml:"enable_xdebug,omitempty"`
	// Varnish puts a varnish container in front of the web server.
	Varnish        bool   `yaml:"varnish,omitempty"`
	VarnishVersion string `yaml:"varnish_version,omitempty"`
	// Cron runs CronJobs (or the type's default job) in the web container.
	Cron     bool     `yaml:"cron,omitempty"`
	CronJobs []string `yaml:"cron_jobs,omitempty"`
	// PerformanceMode is bind or none; empty = the global value. With bind, VolumeDirs (plus the type's
	// defaults) live in Docker volumes.
	PerformanceMode string   `yaml:"performance_mode,omitempty"`
	VolumeDirs      []string `yaml:"volume_dirs,omitempty"`
	// Environment is set in the web container; .lodev/.env overrides it.
	Environment map[string]string `yaml:"environment,omitempty"`
	// LodevVersionConstraint makes start and restart refuse a lodev outside it, e.g. ">= 1.4.0, < 2".
	LodevVersionConstraint string `yaml:"lodev_version_constraint,omitempty"`
	// Hooks are steps run in the web container on start, stop, db-import and snapshot restore.
	Hooks map[string][]HookStep `yaml:"hooks,omitempty"`
	Root  string                `yaml:"-"`
}

// DefaultVarnishVersion is the Varnish a project runs when varnish_version is unset (Magento 2.4.8's).
const DefaultVarnishVersion = "7.6"

// VarnishImage is the varnish image the project runs.
func (p *Project) VarnishImage() string {
	if p.VarnishVersion != "" {
		return "varnish:" + p.VarnishVersion
	}
	return "varnish:" + DefaultVarnishVersion
}

// DefaultWorkingDir is where the project is mounted when working_dir is unset (as on Magento Cloud).
const DefaultWorkingDir = "/app"

// WorkDir is the project's folder inside the web container.
func (p *Project) WorkDir() string {
	if p.WorkingDir != "" {
		return p.WorkingDir
	}
	return DefaultWorkingDir
}

// IDEServerName is the serverName in PHP_IDE_CONFIG.
func (p *Project) IDEServerName() string {
	if p.PHPIDEConfig != "" {
		return p.PHPIDEConfig
	}
	return p.Name
}

var workDirRe = regexp.MustCompile(`^(/[A-Za-z0-9._-]+)+$`)

// systemDirs hold the container's own files (or lodev's mounts): a project mounted there would break it.
var systemDirs = []string{"/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib64", "/opt", "/proc", "/root",
	"/run", "/sbin", "/sys", "/tmp", "/usr", "/var/lib", "/var/log", "/var/run"}

func checkWorkingDir(d string) error {
	if d == "" {
		return nil
	}
	if !workDirRe.MatchString(d) || path.Clean(d) != d {
		return fmt.Errorf("working_dir %q must be an absolute folder like /app (a-z, 0-9, . _ -)", d)
	}
	for _, s := range systemDirs {
		if d == s || strings.HasPrefix(d, s+"/") {
			return fmt.Errorf("working_dir %q is inside the container's %s", d, s)
		}
	}
	return nil
}

var (
	PerformanceModes = []string{"bind", "none"}
	// hostOS is swapped in tests.
	hostOS      = runtime.GOOS
	volumeDirRe = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)
)

// EffectivePerformanceMode is the project's mode, else the global one, else bind on macOS and none elsewhere.
func (p *Project) EffectivePerformanceMode(g *Global) string {
	switch {
	case p.PerformanceMode != "":
		return p.PerformanceMode
	case g != nil && g.PerformanceMode != "":
		return g.PerformanceMode
	case hostOS == "darwin":
		return "bind"
	}
	return "none"
}

// VolumeDir is a project folder kept in a Docker volume; Seed copies the host folder into a new volume.
type VolumeDir struct {
	Path string
	Seed bool
}

// typeVolumeDirs are the folders a type keeps in volumes (node_modules only with a package.json).
func typeVolumeDirs(typ string) []string {
	if typ == "magento" {
		return []string{"var", "generated", "node_modules"}
	}
	return []string{"node_modules"}
}

// VolumeDirsFor is the folders kept in volumes: the type's defaults, then volume_dirs. Nil when the mode is none.
func (p *Project) VolumeDirsFor(g *Global) []VolumeDir {
	if p.EffectivePerformanceMode(g) != "bind" {
		return nil
	}
	var out []VolumeDir
	for _, d := range typeVolumeDirs(p.Type) {
		if d != "node_modules" || exists(filepath.Join(p.Root, "package.json")) {
			out = append(out, VolumeDir{d, false})
		}
	}
	n := len(out)
	for _, d := range p.VolumeDirs {
		if !slices.ContainsFunc(out[:n], func(v VolumeDir) bool { return v.Path == d }) {
			out = append(out, VolumeDir{d, true})
		}
	}
	return out
}

// VolumeName is the Docker volume for a project folder.
func VolumeName(project, dir string) string {
	return "lodev-" + project + "-" + strings.ReplaceAll(dir, "/", "-")
}

func (p *Project) checkVolumeDirs() error {
	all := typeVolumeDirs(p.Type) // nesting counts node_modules whether or not package.json exists
	for _, d := range p.VolumeDirs {
		if slices.Contains(all, d) {
			continue
		}
		if !volumeDirRe.MatchString(d) || path.Clean(d) != d || d == "." || d == ".." || strings.HasPrefix(d, "../") ||
			d == ".lodev" || strings.HasPrefix(d, ".lodev/") {
			return fmt.Errorf("volume_dirs %q must be a folder inside the project, not .lodev (segments of a-z, 0-9, . _ -, joined by /)", d)
		}
		all = append(all, d)
	}
	for i, a := range all {
		for _, b := range all[i+1:] {
			switch {
			case a == b: // a repeated own entry
			case strings.HasPrefix(a, b+"/"), strings.HasPrefix(b, a+"/"):
				return fmt.Errorf("volume_dirs %q and %q are nested (a folder in a volume cannot contain another)", a, b)
			case VolumeName("", a) == VolumeName("", b):
				return fmt.Errorf("volume_dirs %q and %q map to the same volume", a, b)
			}
		}
	}
	return nil
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
	data, _, err = MergeOverrides(root, data)
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
		p.Domains = []string{p.Name + "." + TLD()}
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

func (p *Project) Validate() error { return p.ValidateTLD(TLD()) }

// ValidateTLD validates p as if tld were configured (config-global --tld checks a move with it).
func (p *Project) ValidateTLD(tld string) error {
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
	if d := p.Docroot; d != "" && !insideProject(d) {
		return fmt.Errorf("docroot %q must be a folder inside the project", d)
	}
	if err := validateHooks(p.Hooks); err != nil {
		return err
	}
	if p.PerformanceMode != "" {
		if err := oneOf("performance_mode", p.PerformanceMode, PerformanceModes); err != nil {
			return err
		}
	}
	if err := p.checkVolumeDirs(); err != nil {
		return err
	}
	if err := checkWorkingDir(p.WorkingDir); err != nil {
		return err
	}
	if strings.ContainsAny(p.PHPIDEConfig, "\r\n") {
		return fmt.Errorf("php_ide_config %q must be one line", p.PHPIDEConfig)
	}
	if v := p.LodevVersionConstraint; v != "" {
		if _, err := lodevupdate.ParseConstraint(v); err != nil {
			return fmt.Errorf("lodev_version_constraint %q is invalid: %w", v, err)
		}
	}
	domainRe := domainRegexp(tld)
	for _, d := range p.Domains {
		if err := checkDomain(d, tld, domainRe); err != nil {
			return err
		}
		if slices.Contains(reservedDomains(tld), d) {
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
	if p.Cron && len(p.CronJobs) == 0 && defaultCronJobs[p.Type] == "" {
		return fmt.Errorf("cron needs cron_jobs (type %s has no default job)", p.Type)
	}
	for i, j := range p.CronJobs {
		f := strings.Fields(j)
		if len(f) < 6 && !(len(f) >= 2 && strings.HasPrefix(f[0], "@")) {
			return fmt.Errorf("cron_jobs[%d] %q needs a schedule and a command", i, j)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(p.Environment)) {
		if err := CheckEnvKey(k); err != nil {
			return err
		}
	}
	if p.Varnish && p.Webserver == "none" {
		return errors.New("varnish needs a webserver (webserver is none)")
	}
	if v := p.VarnishVersion; v != "" && !versionRe.MatchString(v) {
		return fmt.Errorf("varnish_version %q is invalid (e.g. 7.6, 9)", v)
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

// grep -v exits 1 when it drops every line (the usual run): || true keeps supercronic from logging an error.
var defaultCronJobs = map[string]string{
	"magento":   `* * * * * php bin/magento cron:run 2>&1 | grep -v "Ran jobs by schedule" >> var/log/magento.cron.log || true`,
	"laravel":   `* * * * * php artisan schedule:run >> storage/logs/schedule.log 2>&1`,
	"wordpress": `* * * * * wp cron event run --due-now >> wp-content/cron.log 2>&1`,
}

// CronLines is the crontab the web container runs: nil when cron is off.
func (p *Project) CronLines() []string {
	switch {
	case !p.Cron:
		return nil
	case len(p.CronJobs) > 0:
		return p.CronJobs
	case defaultCronJobs[p.Type] != "":
		return []string{defaultCronJobs[p.Type]}
	}
	return nil
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// CheckEnvKey rejects names a shell cannot export and lodev's own LODEV_* variables.
func CheckEnvKey(key string) error {
	if !envKeyRe.MatchString(key) {
		return fmt.Errorf("environment key %q is invalid (A-Z, a-z, 0-9, _; not starting with a digit)", key)
	}
	if strings.HasPrefix(key, "LODEV_") {
		return fmt.Errorf("environment: %s is reserved for lodev", key)
	}
	return nil
}

// ReservedDomains are the shared services' domains, on the configured TLD.
func ReservedDomains() []string { return reservedDomains(TLD()) }

// ReservedDomainsFor is ReservedDomains on an explicit TLD.
func ReservedDomainsFor(tld string) []string { return reservedDomains(tld) }

// CheckHost checks the format of a route host (same rule as domains:); it does not check reservations.
func CheckHost(host, tld string) error { return checkDomain(host, tld, domainRegexp(tld)) }

func reservedDomains(tld string) []string {
	t := "." + tld
	return []string{"mailpit" + t, "phpmyadmin" + t, "rabbitmq" + t, "mongo" + t}
}

func domainRegexp(tld string) *regexp.Regexp {
	return regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+` + regexp.QuoteMeta(tld) + `$`)
}

func checkDomain(d, tld string, re *regexp.Regexp) error {
	if re.MatchString(d) {
		return nil
	}
	err := fmt.Sprintf("domain %q is invalid (lowercase host name ending in .%s)", d, tld)
	// On another TLD (e.g. tld edited by hand): say how to move the projects.
	if i := strings.LastIndex(d, "."); i > 0 && d[i+1:] != tld && CheckTLD(d[i+1:]) == nil {
		err += fmt.Sprintf("; run lodev config-global --tld=%s --from-tld=%s to move every project", tld, d[i+1:])
	}
	return errors.New(err)
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

// ImageTag tags lodev's web images: the lodev version, set by cmd.
var ImageTag = "dev"

// WebserverImage is the base web image, with PHP 8.2-8.5.
func WebserverImage() string { return "lodev/webserver:" + ImageTag }

// RegistryRepo is where stable releases publish the base web image.
const RegistryRepo = "ghcr.io/namnh198/lodev-webserver"

// RegistryImage is the base web image a stable release publishes (pulled instead of built).
func RegistryImage() string { return RegistryRepo + ":" + ImageTag }

// DefaultNodeVersion is the Node.js in the base image unless node_version says otherwise.
const DefaultNodeVersion = "24"

// PHPImage is the image a project on PHP ver runs.
func PHPImage(ver string) string {
	if slices.Contains(BundledPHP, ver) {
		return WebserverImage()
	}
	return WebserverImage() + "-legacy-" + ver
}
