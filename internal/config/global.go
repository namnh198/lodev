package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Global struct {
	Ports struct {
		HTTP      int    `yaml:"http"`
		HTTPS     int    `yaml:"https"`
		Dashboard int    `yaml:"dashboard"`
		MySQL     string `yaml:"mysql"`
		MariaDB   string `yaml:"mariadb"`
		MongoDB   string `yaml:"mongodb"`
	} `yaml:"ports"`
	Services map[string]bool   `yaml:"services"`
	Versions map[string]string `yaml:"versions"`
	Memory   struct {
		OpenSearch string `yaml:"opensearch"`
	} `yaml:"memory"`
	UpdateCheck bool `yaml:"update_check"`
	// Run the system docker compose / docker build instead of lodev's pinned binaries.
	UseDockerComposeSystem bool `yaml:"use_docker_compose_system"`
	UseDockerBuildxSystem  bool `yaml:"use_docker_buildx_system"`
	// DataDir holds each project's home and, with MountVolume, the shared services' data.
	DataDir     string            `yaml:"data_dir,omitempty"`
	MountVolume bool              `yaml:"mount_volume"`
	MountPaths  map[string]string `yaml:"mount_paths,omitempty"`
}

var (
	VersionedServices = []string{"mysql", "mariadb", "redis", "opensearch", "rabbitmq", "mongodb"}
	DefaultVersions   = map[string]string{"mysql": "8.0", "mariadb": "11.4", "redis": "7", "opensearch": "2", "rabbitmq": "4", "mongodb": "8"}
	// DataServices keep data in a volume (or, with mount_volume, a folder).
	DataServices = []string{"mysql", "mariadb", "redis", "opensearch", "rabbitmq", "mongodb"}
	versionRe    = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
)

// Image is the image of a versioned shared service at its configured version.
func (g *Global) Image(svc string) string {
	v := g.Versions[svc]
	switch svc {
	case "redis":
		return "redis:" + v + "-alpine"
	case "opensearch":
		return "opensearchproject/opensearch:" + v
	case "rabbitmq":
		return "rabbitmq:" + v + "-management"
	case "mongodb":
		return "mongo:" + v
	}
	return svc + ":" + v
}

// expandPath resolves a leading ~/ to the user's home.
func expandPath(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, rest)
	}
	return p
}

func checkPath(key, p string) error {
	if !filepath.IsAbs(p) && !strings.HasPrefix(p, "~/") {
		return fmt.Errorf("%s %q must be an absolute path or start with ~/", key, p)
	}
	return nil
}

// DataPath is data_dir expanded, or ~/.lodev/data.
func (g *Global) DataPath() string {
	if g.DataDir == "" {
		return filepath.Join(Home(), "data")
	}
	return expandPath(g.DataDir)
}

// MountPath is the folder svc keeps its data in, or "" for its named volume.
func (g *Global) MountPath(svc string) string {
	if !g.MountVolume || !slices.Contains(DataServices, svc) {
		return ""
	}
	if p := g.MountPaths[svc]; p != "" {
		return expandPath(p)
	}
	return filepath.Join(g.DataPath(), svc)
}

// GlobalPath is the global config file.
func GlobalPath() string { return filepath.Join(Home(), "config.yaml") }

func defaultGlobal() *Global {
	g := &Global{Services: map[string]bool{}, Versions: maps.Clone(DefaultVersions)}
	g.Ports.HTTP, g.Ports.HTTPS, g.Ports.Dashboard = 80, 443, 8080
	g.Memory.OpenSearch = "2g"
	g.UpdateCheck = true
	for _, s := range SharedServices {
		g.Services[s] = !slices.Contains(DefaultOff, s)
	}
	return g
}

// LoadGlobal reads ~/.lodev/config.yaml over the defaults; a missing file means defaults.
func LoadGlobal() (*Global, error) {
	data, err := os.ReadFile(GlobalPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	g, err := ParseGlobal(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", GlobalPath(), err)
	}
	return g, nil
}

// ParseGlobal decodes a global config over the defaults and validates it.
// A *ValidationError comes with the decoded config; a YAML error with nil.
func ParseGlobal(data []byte) (*Global, error) {
	g := defaultGlobal()
	// yaml.v3 merges into the pre-filled struct and map, so absent keys keep defaults.
	if err := yaml.Unmarshal(data, g); err != nil {
		return nil, err
	}
	// An empty `versions:` or `services:` block decodes as null and drops the whole default map.
	d := defaultGlobal()
	if g.Versions == nil {
		g.Versions = d.Versions
	}
	if g.Services == nil {
		g.Services = d.Services
	}
	if err := g.Validate(); err != nil {
		return g, &ValidationError{err}
	}
	return g, nil
}

func SaveGlobal(g *Global) error {
	data, err := yaml.Marshal(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(Home(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(GlobalPath(), data, 0o644)
}

func (g *Global) Validate() error {
	for _, p := range []struct{ key, val string }{{"mysql", g.Ports.MySQL}, {"mariadb", g.Ports.MariaDB}, {"mongodb", g.Ports.MongoDB}} {
		if p.val == "" {
			continue
		}
		if n, err := strconv.Atoi(p.val); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("ports.%s %q is not a valid port", p.key, p.val)
		}
	}
	if g.Ports.MariaDB != "" && g.Ports.MariaDB == g.Ports.MySQL {
		return errors.New("ports.mariadb must differ from ports.mysql")
	}
	ports := []struct{ key, val string }{
		{"http", strconv.Itoa(g.Ports.HTTP)}, {"https", strconv.Itoa(g.Ports.HTTPS)},
		{"dashboard", strconv.Itoa(g.Ports.Dashboard)}, {"mysql", g.Ports.MySQL}, {"mariadb", g.Ports.MariaDB},
		{"mongodb", g.Ports.MongoDB},
	}
	for _, p := range []struct {
		key string
		n   int
	}{{"http", g.Ports.HTTP}, {"https", g.Ports.HTTPS}, {"dashboard", g.Ports.Dashboard}} {
		if p.n < 1 || p.n > 65535 {
			return fmt.Errorf("ports.%s %d is not a valid port", p.key, p.n)
		}
	}
	for i, a := range ports {
		for _, b := range ports[i+1:] {
			if a.val != "" && a.val == b.val && !(a.key == "mysql" && b.key == "mariadb") { // reported above
				return fmt.Errorf("ports.%s and ports.%s are both %s", a.key, b.key, a.val)
			}
		}
	}
	for s := range g.Services {
		if err := oneOf("services", s, SharedServices); err != nil {
			return err
		}
	}
	for _, s := range slices.Sorted(maps.Keys(g.Versions)) {
		if !slices.Contains(VersionedServices, s) {
			return fmt.Errorf("versions: unknown service %q (allowed: %s)", s, strings.Join(VersionedServices, ", "))
		}
		if !versionRe.MatchString(g.Versions[s]) {
			return fmt.Errorf("versions.%s %q is not a version (e.g. 8.0)", s, g.Versions[s])
		}
	}
	if g.DataDir != "" {
		if err := checkPath("data_dir", g.DataDir); err != nil {
			return err
		}
	}
	for _, s := range slices.Sorted(maps.Keys(g.MountPaths)) {
		if !slices.Contains(DataServices, s) {
			return fmt.Errorf("mount_paths: unknown service %q (allowed: %s)", s, strings.Join(DataServices, ", "))
		}
		if err := checkPath("mount_paths."+s, g.MountPaths[s]); err != nil {
			return err
		}
	}
	_, err := HeapSize(g.Memory.OpenSearch)
	return err
}

// PMAHosts is phpMyAdmin's server list: the enabled databases, or mysql when none is.
func (g *Global) PMAHosts() string {
	var hosts []string
	for _, db := range []string{"mysql", "mariadb"} {
		if g.Services[db] {
			hosts = append(hosts, db)
		}
	}
	if len(hosts) == 0 {
		return "mysql"
	}
	return strings.Join(hosts, ",")
}

func (g *Global) EnabledServices() []string {
	var out []string
	for _, s := range SharedServices {
		if g.Services[s] {
			out = append(out, s)
		}
	}
	return out
}

// HeapSize returns the OpenSearch JVM heap (half of the container limit).
func HeapSize(limit string) (string, error) {
	heaps := map[string]string{"1g": "512m", "2g": "1g", "3g": "1536m", "4g": "2g"}
	if h, ok := heaps[limit]; ok {
		return h, nil
	}
	return "", fmt.Errorf("memory.opensearch %q is invalid (allowed: 1g, 2g, 3g, 4g)", limit)
}
