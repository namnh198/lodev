package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"lodev/internal/config"
	"lodev/internal/hosts"
	"lodev/internal/registry"
)

// tldMove is one project's planned switch to the new TLD.
type tldMove struct {
	name    string
	p       *config.Project
	doc     *config.Doc // nil: the config has no domains key (its default follows the TLD)
	domains []string
}

// retargetTLD moves every registered project from oldTLD to newTLD: their configs and the
// registry, the global tld, /etc/hosts, and whatever runs. Everything is checked before anything
// is written; a project that cannot move stops the whole change.
func retargetTLD(oldTLD, newTLD string, out io.Writer) error {
	if err := config.CheckTLD(newTLD); err != nil {
		return err
	}
	if oldTLD == newTLD {
		fmt.Fprintf(out, "tld is already .%s\n", newTLD)
		return nil
	}
	if newTLD == "local" && setupOS == "darwin" {
		fmt.Fprintln(out, ".local is resolved by mDNS on macOS: lookups can be slow or skip /etc/hosts; .test avoids it")
	}
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	var moves []tldMove
	for _, name := range reg.Names() {
		m, err := planTLDMove(name, reg[name].Path, oldTLD, newTLD)
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "skipped %s: %s is gone\n", name, config.ProjectPath(reg[name].Path))
			continue
		}
		if err != nil {
			return err
		}
		moves = append(moves, m)
	}

	for _, m := range moves {
		if m.doc != nil {
			m.doc.Set([]string{"domains"}, m.domains)
			data, err := m.doc.Bytes()
			if err == nil {
				err = m.doc.Write(data)
			}
			if err != nil {
				return err
			}
		}
		reg[m.name] = registry.Entry{Path: m.p.Root, Domains: m.p.WebDomains()}
	}
	if err := reg.Save(); err != nil {
		return err
	}
	g, err := config.OpenDoc(config.GlobalPath())
	if err != nil {
		return err
	}
	g.Set([]string{"tld"}, newTLD)
	data, err := g.Bytes()
	if err == nil {
		err = g.Write(data)
	}
	if err != nil {
		return err
	}

	if err := hosts.Apply("lodev-services", config.ReservedDomains()); err != nil {
		return err
	}
	for _, m := range moves {
		if err := hosts.Apply(m.name, m.p.WebDomains()); err != nil {
			return err
		}
	}
	if containerRunning("lodev-traefik") {
		if err := servicesUpFn(); err != nil {
			return err
		}
	}
	running, err := runningProjects()
	if err != nil {
		return err
	}
	for _, m := range moves {
		if containsString(running, "lodev-"+m.name+"-web") {
			if err := restartFn(m.p); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(out, "TLD changed .%s → .%s: %d project(s) updated\n", oldTLD, newTLD, len(moves))
	return nil
}

// planTLDMove reads one project and works out its domains on newTLD, without writing.
func planTLDMove(name, root, oldTLD, newTLD string) (tldMove, error) {
	data, err := os.ReadFile(config.ProjectPath(root))
	if err != nil {
		return tldMove{}, err
	}
	var raw struct {
		Domains []string `yaml:"domains"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return tldMove{}, fmt.Errorf("project %s: %w", name, err)
	}
	m := tldMove{name: name}
	for _, d := range raw.Domains {
		base, ok := strings.CutSuffix(d, "."+oldTLD)
		if !ok {
			return tldMove{}, fmt.Errorf("project %s: domain %q does not end in .%s", name, d, oldTLD)
		}
		m.domains = append(m.domains, base+"."+newTLD)
	}
	var vErr *config.ValidationError
	p, err := config.ParseProject(root, data) // validated below against newTLD, not the current one
	if err != nil && !errors.As(err, &vErr) {
		return tldMove{}, fmt.Errorf("project %s: %w", name, err)
	}
	if len(raw.Domains) > 0 {
		p.Domains = m.domains
		if m.doc, err = config.OpenDoc(config.ProjectPath(root)); err != nil {
			return tldMove{}, err
		}
	} else {
		p.Domains = []string{p.Name + "." + newTLD}
	}
	if err := p.ValidateTLD(newTLD); err != nil {
		return tldMove{}, fmt.Errorf("project %s: %w", name, err)
	}
	m.p = p
	return m, nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
