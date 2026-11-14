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
	"lodev/internal/ui"
)

// tldMove is one project's planned switch to the new TLD.
type tldMove struct {
	name    string
	p       *config.Project
	doc     *config.Doc // nil: the config has no domains key (its default follows the TLD)
	domains []string
	// overrides are the .lodev/config.*.yaml files that set domains, already edited
	overrides []*config.Doc
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
	var skipped []string
	for _, name := range reg.Names() {
		m, err := planTLDMove(name, reg[name].Path, oldTLD, newTLD)
		if errors.Is(err, fs.ErrNotExist) {
			ui.Warn("skipped %s: %s is gone (its /etc/hosts entry moves anyway)", name, config.ProjectPath(reg[name].Path))
			skipped = append(skipped, name)
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
		}
		for _, d := range append(m.overrides, m.doc) {
			if d == nil {
				continue
			}
			data, err := d.Bytes()
			if err == nil {
				err = d.Write(data)
			}
			if err != nil {
				return err
			}
		}
		reg[m.name] = registry.Entry{Path: m.p.Root, Domains: m.p.WebDomains()}
	}
	for _, name := range skipped { // no config to read: move what the registry remembers
		e := reg[name]
		for i, d := range e.Domains {
			if base, ok := strings.CutSuffix(d, "."+oldTLD); ok {
				e.Domains[i] = base + "." + newTLD
			}
		}
		reg[name] = e
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

	// The configs are on newTLD now: from here on a failure is finished by rerunning with --from-tld.
	rerun := fmt.Sprintf("rerun: lodev config-global --tld=%s --from-tld=%s", newTLD, oldTLD)
	blocks := []hosts.Block{{Name: "lodev-services", Domains: config.ReservedDomains()}}
	for _, m := range moves {
		blocks = append(blocks, hosts.Block{Name: m.name, Domains: m.p.WebDomains()})
	}
	for _, name := range skipped {
		blocks = append(blocks, hosts.Block{Name: name, Domains: reg[name].Domains})
	}
	if err := hosts.ApplyAll(blocks); err != nil { // one write: one sudo prompt, one backup of the original
		return fmt.Errorf("%w; %s", err, rerun)
	}
	var errs []error
	if containerRunning("lodev-traefik") {
		if err := servicesUpFn(); err != nil {
			errs = append(errs, err)
		}
	}
	running, err := runningProjects()
	if err != nil { // Docker down: nothing runs, so nothing needs a restart
		fmt.Fprintf(out, "could not list running projects (%v); start them to use the new domains\n", err)
	}
	for _, m := range moves {
		if containsString(running, "lodev-"+m.name+"-web") {
			if err := restartFn(m.p); err != nil { // keep going: the others still need their new domains
				errs = append(errs, fmt.Errorf("restart %s: %w", m.name, err))
			}
		}
	}
	fmt.Fprintf(out, "TLD changed .%s → .%s: %d project(s) updated\n", oldTLD, newTLD, len(moves))
	if len(errs) > 0 {
		return fmt.Errorf("%w; %s", errors.Join(errs...), rerun)
	}
	return nil
}

// planTLDMove reads one project, overrides included, and works out its domains on newTLD,
// without writing.
func planTLDMove(name, root, oldTLD, newTLD string) (tldMove, error) {
	data, err := os.ReadFile(config.ProjectPath(root))
	if err != nil {
		return tldMove{}, err
	}
	moveDomains := func(file string) (domains []string, found bool, err error) {
		var raw struct {
			Domains []string `yaml:"domains"`
		}
		if b, err := os.ReadFile(file); err != nil {
			return nil, false, err
		} else if err := yaml.Unmarshal(b, &raw); err != nil {
			return nil, false, fmt.Errorf("project %s: %w", name, err)
		}
		for _, d := range raw.Domains {
			if strings.HasSuffix(d, "."+newTLD) { // moved by an earlier run that failed later on
				domains = append(domains, d)
				continue
			}
			base, ok := strings.CutSuffix(d, "."+oldTLD)
			if !ok {
				return nil, false, fmt.Errorf("project %s: domain %q does not end in .%s", name, d, oldTLD)
			}
			domains = append(domains, base+"."+newTLD)
		}
		return domains, len(raw.Domains) > 0, nil
	}
	m := tldMove{name: name}
	own, hasDomains, err := moveDomains(config.ProjectPath(root))
	if err != nil {
		return tldMove{}, err
	}
	effective := own // the last file that sets domains wins, as when the project is read
	files, err := config.OverrideFiles(root)
	if err != nil {
		return tldMove{}, fmt.Errorf("project %s: %w", name, err)
	}
	for _, f := range files {
		ds, found, err := moveDomains(f)
		if err != nil {
			return tldMove{}, err
		}
		if !found {
			continue
		}
		d, err := config.OpenDoc(f)
		if err != nil {
			return tldMove{}, err
		}
		d.Set([]string{"domains"}, ds)
		m.overrides = append(m.overrides, d)
		effective = ds
	}
	merged, _, err := config.MergeOverrides(root, data)
	if err != nil {
		return tldMove{}, fmt.Errorf("project %s: %w", name, err)
	}
	var vErr *config.ValidationError
	p, err := config.ParseProject(root, merged) // validated below against newTLD, not the current one
	if err != nil && !errors.As(err, &vErr) {
		return tldMove{}, fmt.Errorf("project %s: %w", name, err)
	}
	if hasDomains {
		m.domains = own
		if m.doc, err = config.OpenDoc(config.ProjectPath(root)); err != nil {
			return tldMove{}, err
		}
	}
	if len(effective) > 0 {
		p.Domains = effective
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
