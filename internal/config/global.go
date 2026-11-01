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
	} `yaml:"ports"`
	Services map[string]bool   `yaml:"services"`
	Versions map[string]string `yaml:"versions"`
	Memory   struct {
		OpenSearch string `yaml:"opensearch"`
	} `yaml:"memory"`
}

var (
	VersionedServices = []string{"mysql", "mariadb", "redis", "opensearch", "rabbitmq"}
	DefaultVersions   = map[string]string{"mysql": "8.0", "mariadb": "11.4", "redis": "7", "opensearch": "2", "rabbitmq": "4"}
	versionRe         = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,2}$`)
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
	}
	return svc + ":" + v
}

func globalPath() string { return filepath.Join(Home(), "config.yaml") }

func defaultGlobal() *Global {
	g := &Global{Services: map[string]bool{}, Versions: maps.Clone(DefaultVersions)}
	g.Ports.HTTP, g.Ports.HTTPS, g.Ports.Dashboard = 80, 443, 8080
	g.Memory.OpenSearch = "2g"
	for _, s := range SharedServices {
		g.Services[s] = !slices.Contains(DefaultOff, s)
	}
	return g
}

// LoadGlobal reads ~/.lodev/config.yaml over the defaults; a missing file means defaults.
func LoadGlobal() (*Global, error) {
	g := defaultGlobal()
	data, err := os.ReadFile(globalPath())
	if errors.Is(err, os.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	// yaml.v3 merges into the pre-filled struct and map, so absent keys keep defaults.
	if err := yaml.Unmarshal(data, g); err != nil {
		return nil, fmt.Errorf("%s: %w", globalPath(), err)
	}
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", globalPath(), err)
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
	return os.WriteFile(globalPath(), data, 0o644)
}

func (g *Global) Validate() error {
	if g.Ports.MySQL != "" {
		if n, err := strconv.Atoi(g.Ports.MySQL); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("ports.mysql %q is not a valid port", g.Ports.MySQL)
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
	_, err := HeapSize(g.Memory.OpenSearch)
	return err
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
