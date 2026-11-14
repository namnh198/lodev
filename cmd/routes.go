package cmd

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"lodev/internal/config"
)

// virtualService is a compose service that declares a Traefik route through VIRTUAL_HOST / VIRTUAL_PORT.
type virtualService struct {
	Service, Container string
	Hosts              []string
	Port               int
}

// virtualServices reads the routes from a merged compose file (as `docker compose config` prints it).
// forbidden names the services with their own routes (a project's web and varnish); a non-empty list
// also means the file is a project's, whose hosts must not take a shared service's domain.
func virtualServices(full []byte, composeProject, tld string, forbidden []string) ([]virtualService, error) {
	var doc struct {
		Services map[string]struct {
			ContainerName string            `yaml:"container_name"`
			Environment   map[string]string `yaml:"environment"`
			Networks      map[string]any    `yaml:"networks"`
			Deploy        struct {
				Replicas int `yaml:"replicas"`
			} `yaml:"deploy"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(full, &doc); err != nil {
		return nil, fmt.Errorf("compose config: %w", err)
	}
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []virtualService
	used := map[string]string{} // host -> service
	for _, name := range names {
		s := doc.Services[name]
		raw, ok := s.Environment["VIRTUAL_HOST"]
		if !ok {
			continue
		}
		if slices.Contains(forbidden, name) {
			return nil, fmt.Errorf("service %s: VIRTUAL_HOST is not supported on %s; set domains: in .lodev/config.yaml", name, name)
		}
		var hosts []string
		for _, h := range strings.Split(raw, ",") {
			h = strings.TrimSpace(h)
			if h == "" {
				return nil, fmt.Errorf("service %s: VIRTUAL_HOST %q has an empty entry", name, raw)
			}
			if err := config.CheckHost(h, tld); err != nil {
				return nil, fmt.Errorf("service %s: VIRTUAL_HOST: %w", name, err)
			}
			if len(forbidden) > 0 && slices.Contains(config.ReservedDomainsFor(tld), h) {
				return nil, fmt.Errorf("service %s: VIRTUAL_HOST %q is reserved by lodev", name, h)
			}
			if other, dup := used[h]; dup {
				return nil, fmt.Errorf("service %s: VIRTUAL_HOST %q is already used by service %s", name, h, other)
			}
			used[h] = name
			hosts = append(hosts, h)
		}
		port := 80
		if p, ok := s.Environment["VIRTUAL_PORT"]; ok {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("service %s: VIRTUAL_PORT %q must be an integer from 1 to 65535", name, p)
			}
			port = n
		}
		if _, ok := s.Networks["lodev"]; !ok {
			return nil, fmt.Errorf("service %s: VIRTUAL_HOST needs the lodev network (Traefik reaches it there): add networks: [lodev, default]", name)
		}
		if s.Deploy.Replicas > 1 {
			return nil, fmt.Errorf("service %s: VIRTUAL_HOST does not support deploy.replicas > 1 (one backend only)", name)
		}
		container := s.ContainerName
		if container == "" {
			container = composeProject + "-" + name + "-1"
		}
		out = append(out, virtualService{Service: name, Container: container, Hosts: hosts, Port: port})
	}
	return out, nil
}
