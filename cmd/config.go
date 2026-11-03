package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

const notAProject = "no lodev project here (.lodev/config.yaml not found). Create it first: lodev create"

// change sets key to value, or deletes key when del is set.
type change struct {
	key   []string
	value any
	del   bool
}

func applyChanges(d *config.Doc, changes []change) {
	for _, c := range changes {
		if c.del {
			d.Delete(c.key)
		} else {
			d.Set(c.key, c.value)
		}
	}
}

func shown(v string) string {
	if v == "" {
		return "(unset)"
	}
	return v
}

// diffLines lists "key: old → new" for every key whose value differs; rows are {key, value}.
func diffLines(old, new [][2]string) []string {
	was := map[string]string{}
	for _, r := range old {
		was[r[0]] = r[1]
	}
	var out []string
	for _, r := range new {
		if was[r[0]] != r[1] {
			out = append(out, fmt.Sprintf("%s: %s → %s", r[0], shown(was[r[0]]), shown(r[1])))
		}
	}
	return out
}

func printDiff(w io.Writer, diff []string, hint string) {
	if len(diff) == 0 {
		fmt.Fprintln(w, "nothing changed")
		return
	}
	for _, l := range diff {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w, hint)
}

// listForDiff shows a list as [a b], or "" when empty.
func listForDiff(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return "[" + strings.Join(l, " ") + "]"
}

func listForTable(l []string) string { return strings.Join(l, "\n") }

// projectValues is the effective project config as {key, value} rows in file order.
func projectValues(p *config.Project, list func([]string) string) [][2]string {
	return [][2]string{
		{"name", p.Name}, {"type", p.Type}, {"php", p.PHP}, {"webserver", p.Webserver},
		{"docroot", p.Docroot}, {"domains", list(p.Domains)}, {"services", list(p.Services)},
		{"nodejs", p.NodeJS}, {"php_extensions", list(p.PHPExtensions)},
		{"packages", list(p.Packages)}, {"user", p.User},
	}
}

func valueRows(rows [][2]string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = []string{r[0], r[1]}
	}
	return out
}

func anyFlagChanged(c *cobra.Command) bool {
	changed := false
	c.Flags().Visit(func(*pflag.Flag) { changed = true })
	return changed
}

// editList applies --<name>, --add-<name>, --remove-<name> (in that order) to cur.
func editList(c *cobra.Command, name string, cur []string) ([]string, bool) {
	fl := c.Flags()
	if !fl.Changed(name) && !fl.Changed("add-"+name) && !fl.Changed("remove-"+name) {
		return nil, false
	}
	out := slices.Clone(cur)
	if fl.Changed(name) {
		out, _ = fl.GetStringSlice(name)
	}
	add, _ := fl.GetStringSlice("add-" + name)
	for _, v := range add {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	rm, _ := fl.GetStringSlice("remove-" + name)
	out = slices.DeleteFunc(out, func(v string) bool { return slices.Contains(rm, v) })
	return out, true
}

// projectChanges turns the changed flags into changes; lists start from old's effective values.
func projectChanges(c *cobra.Command, old *config.Project) ([]change, error) {
	fl := c.Flags()
	var out []change
	for _, s := range []struct{ flag, key string }{
		{"type", "type"}, {"php-version", "php"}, {"webserver", "webserver"},
		{"docroot", "docroot"}, {"nodejs-version", "nodejs"}, {"user", "user"},
	} {
		if !fl.Changed(s.flag) {
			continue
		}
		v, _ := fl.GetString(s.flag)
		switch {
		case s.key == "docroot" && (v == "" || v == "."):
			out = append(out, change{key: []string{"docroot"}, value: ""})
		case v == "" && (s.key == "type" || s.key == "php"):
			return nil, fmt.Errorf("--%s cannot be empty", s.flag)
		case v == "":
			out = append(out, change{key: []string{s.key}, del: true})
		default:
			out = append(out, change{key: []string{s.key}, value: v})
		}
	}
	for _, l := range []struct {
		flag, key string
		cur       []string
	}{
		{"domains", "domains", old.Domains}, {"services", "services", old.Services},
		{"php-extensions", "php_extensions", old.PHPExtensions}, {"packages", "packages", old.Packages},
	} {
		v, ok := editList(c, l.flag, l.cur)
		switch {
		case !ok:
		case len(v) == 0 && l.key == "services":
			out = append(out, change{key: []string{"services"}, value: []string{}})
		case len(v) == 0:
			out = append(out, change{key: []string{l.key}, del: true})
		default:
			out = append(out, change{key: []string{l.key}, value: v})
		}
	}
	return out, nil
}

// loadProjectForEdit reads the project config; a file that only fails validation is returned
// as is, so the command can fix it.
func loadProjectForEdit(root string) (*config.Project, error) {
	data, err := os.ReadFile(config.ProjectPath(root))
	if err != nil {
		return nil, err
	}
	p, err := config.ParseProject(root, data)
	if ve := (*config.ValidationError)(nil); err != nil && !errors.As(err, &ve) {
		return nil, fmt.Errorf("%s: %w", config.ProjectPath(root), err)
	}
	return p, nil
}

// applyProjectConfig writes changes to old's config file when the result is valid and differs.
func applyProjectConfig(old *config.Project, changes []change) (diff []string, p *config.Project, err error) {
	d, err := config.OpenDoc(config.ProjectPath(old.Root))
	if err != nil {
		return nil, nil, err
	}
	applyChanges(d, changes)
	data, err := d.Bytes()
	if err != nil {
		return nil, nil, err
	}
	if p, err = config.ParseProject(old.Root, data); err != nil {
		return nil, nil, fmt.Errorf("not saved: %w", err)
	}
	reg, err := registry.Load()
	if err != nil {
		return nil, nil, err
	}
	if err := reg.Conflict(p); err != nil {
		return nil, nil, fmt.Errorf("not saved: %w", err)
	}
	diff = diffLines(projectValues(old, listForDiff), projectValues(p, listForDiff))
	if len(diff) == 0 {
		return nil, p, nil
	}
	return diff, p, d.Write(data)
}

func projectWarnings(c *cobra.Command, p *config.Project) {
	if c.Flags().Changed("docroot") && p.Docroot != "" {
		if _, err := os.Stat(filepath.Join(p.Root, p.Docroot)); errors.Is(err, os.ErrNotExist) {
			ui.Warn("docroot %s does not exist yet", p.Docroot)
		}
	}
	if c.Flags().Changed("user") && p.User != "" {
		if name := config.SanitizeUser(p.User); name != p.User {
			ui.Warn("user %q will be %s in the container", p.User, name)
		}
	}
}

func newConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:           "config",
		Short:         "Show or change the project's .lodev/config.yaml",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			root, err := config.FindRoot(wd)
			if err != nil {
				return errors.New(notAProject)
			}
			if !anyFlagChanged(c) {
				p, err := loadProjectForEdit(root)
				if err != nil {
					return err
				}
				printTable(c.OutOrStdout(), []string{"KEY", "VALUE"}, valueRows(projectValues(p, listForTable)))
				return nil
			}
			return withLock(func() error {
				old, err := loadProjectForEdit(root)
				if err != nil {
					return err
				}
				changes, err := projectChanges(c, old)
				if err != nil {
					return err
				}
				diff, p, err := applyProjectConfig(old, changes)
				if err != nil {
					return err
				}
				hint := "Applies on the next lodev start"
				if status(p.Name) == "running" {
					hint = "Run: lodev restart to apply"
				}
				printDiff(c.OutOrStdout(), diff, hint)
				if len(diff) > 0 {
					projectWarnings(c, p)
				}
				return nil
			})
		},
	}
	f := c.Flags()
	f.String("type", "", "laravel | wordpress | magento | php")
	f.String("php-version", "", "PHP version ("+strings.Join(config.SupportedPHP, ", ")+")")
	f.String("webserver", "", `nginx | apache | none ("" = nginx)`)
	f.String("docroot", "", `document root relative to the project; "." or "" for the project root`)
	f.String("nodejs-version", "", `Node.js version: lts | latest | current | 20 | 20.11 ("" removes Node.js)`)
	f.String("user", "", `shell user in the web container ("" = your host username)`)
	for _, l := range []struct{ name, what string }{
		{"domains", "domains (.test)"}, {"services", "shared services"},
		{"php-extensions", "PHP extensions"}, {"packages", "apt packages"},
	} {
		f.StringSlice(l.name, nil, "replace the "+l.what)
		f.StringSlice("add-"+l.name, nil, "add "+l.what)
		f.StringSlice("remove-"+l.name, nil, "remove "+l.what)
	}
	return c
}

// globalValues is the effective global config as {key, value} rows.
func globalValues(g *config.Global) [][2]string {
	rows := [][2]string{
		{"ports.http", strconv.Itoa(g.Ports.HTTP)}, {"ports.https", strconv.Itoa(g.Ports.HTTPS)},
		{"ports.dashboard", strconv.Itoa(g.Ports.Dashboard)}, {"ports.mysql", g.Ports.MySQL},
		{"ports.mariadb", g.Ports.MariaDB},
	}
	for _, s := range config.SharedServices {
		rows = append(rows, [2]string{"services." + s, strconv.FormatBool(g.Services[s])})
	}
	for _, s := range config.VersionedServices {
		rows = append(rows, [2]string{"versions." + s, g.Versions[s]})
	}
	return append(rows, [2]string{"memory.opensearch", g.Memory.OpenSearch})
}

// loadGlobalForEdit reads the global config; one that only fails validation is returned as is.
func loadGlobalForEdit() (*config.Global, error) {
	data, err := os.ReadFile(config.GlobalPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	g, err := config.ParseGlobal(data)
	if ve := (*config.ValidationError)(nil); err != nil && !errors.As(err, &ve) {
		return nil, fmt.Errorf("%s: %w", config.GlobalPath(), err)
	}
	return g, nil
}

// applyGlobalConfig writes changes to the global config when the result is valid and differs.
func applyGlobalConfig(old *config.Global, changes []change) (diff []string, g *config.Global, err error) {
	d, err := config.OpenDoc(config.GlobalPath())
	if err != nil {
		return nil, nil, err
	}
	applyChanges(d, changes)
	data, err := d.Bytes()
	if err != nil {
		return nil, nil, err
	}
	if g, err = config.ParseGlobal(data); err != nil {
		return nil, nil, fmt.Errorf("not saved: %w", err)
	}
	if diff = diffLines(globalValues(old), globalValues(g)); len(diff) == 0 {
		return nil, g, nil
	}
	return diff, g, d.Write(data)
}

// warnProjectsUsing warns about every registered project that uses svc.
func warnProjectsUsing(svc string) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	for _, name := range reg.Names() {
		if p, err := config.LoadProject(reg[name].Path); err == nil && p.Uses(svc) {
			ui.Warn("project %s uses %s", name, svc)
		}
	}
	return nil
}

func checkServices(names []string) error {
	for _, s := range names {
		if !slices.Contains(config.SharedServices, s) {
			return fmt.Errorf("unknown service %q (allowed: %s)", s, strings.Join(config.SharedServices, ", "))
		}
	}
	return nil
}

// globalChanges turns the changed flags into changes.
func globalChanges(c *cobra.Command) ([]change, error) {
	fl := c.Flags()
	var out []change
	set := func(key []string, flag string, v any) {
		if s, _ := fl.GetString(flag); s == "" {
			out = append(out, change{key: key, del: true})
		} else {
			out = append(out, change{key: key, value: v})
		}
	}
	for _, p := range []string{"http", "https", "dashboard"} {
		flag := p + "-port"
		if !fl.Changed(flag) {
			continue
		}
		v, _ := fl.GetString(flag)
		n, err := strconv.Atoi(v)
		if v != "" && err != nil {
			return nil, fmt.Errorf("--%s %q is not a number", flag, v)
		}
		set([]string{"ports", p}, flag, n)
	}
	for _, p := range []string{"mysql", "mariadb"} {
		if flag := p + "-port"; fl.Changed(flag) {
			v, _ := fl.GetString(flag)
			set([]string{"ports", p}, flag, v)
		}
	}
	for _, s := range config.VersionedServices {
		if flag := s + "-version"; fl.Changed(flag) {
			v, _ := fl.GetString(flag)
			set([]string{"versions", s}, flag, v)
		}
	}
	if fl.Changed("opensearch-memory") {
		v, _ := fl.GetString("opensearch-memory")
		set([]string{"memory", "opensearch"}, "opensearch-memory", v)
	}
	on, _ := fl.GetStringSlice("enable-services")
	off, _ := fl.GetStringSlice("disable-services")
	if err := checkServices(append(slices.Clone(on), off...)); err != nil {
		return nil, err
	}
	for _, s := range on {
		if slices.Contains(off, s) {
			return nil, fmt.Errorf("--enable-services and --disable-services both name %s", s)
		}
		out = append(out, change{key: []string{"services", s}, value: true})
	}
	for _, s := range off {
		out = append(out, change{key: []string{"services", s}, value: false})
	}
	return out, nil
}

func newConfigGlobalCmd() *cobra.Command {
	c := &cobra.Command{
		Use:           "config-global",
		Short:         "Show or change the global ~/.lodev/config.yaml",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			if !anyFlagChanged(c) {
				g, err := loadGlobalForEdit()
				if err != nil {
					return err
				}
				printTable(c.OutOrStdout(), []string{"KEY", "VALUE"}, valueRows(globalValues(g)))
				return nil
			}
			changes, err := globalChanges(c)
			if err != nil {
				return err
			}
			return withLock(func() error {
				old, err := loadGlobalForEdit()
				if err != nil {
					return err
				}
				diff, _, err := applyGlobalConfig(old, changes)
				if err != nil {
					return err
				}
				printDiff(c.OutOrStdout(), diff, "Run: lodev services up to apply")
				if len(diff) == 0 {
					return nil
				}
				off, _ := c.Flags().GetStringSlice("disable-services")
				for _, s := range off {
					if err := warnProjectsUsing(s); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	f := c.Flags()
	f.String("http-port", "", `host HTTP port ("" = 80)`)
	f.String("https-port", "", `host HTTPS port ("" = 443)`)
	f.String("dashboard-port", "", `Traefik dashboard port ("" = 8080)`)
	f.String("mysql-port", "", `publish MySQL on this host port ("" = not published)`)
	f.String("mariadb-port", "", `publish MariaDB on this host port ("" = not published)`)
	for _, s := range config.VersionedServices {
		f.String(s+"-version", "", fmt.Sprintf(`%s version, e.g. %s ("" = default)`, s, config.DefaultVersions[s]))
	}
	f.String("opensearch-memory", "", `OpenSearch memory limit: 1g | 2g | 3g | 4g ("" = 2g)`)
	f.StringSlice("enable-services", nil, "turn shared services on ("+strings.Join(config.SharedServices, ", ")+")")
	f.StringSlice("disable-services", nil, "turn shared services off (data is kept)")
	return c
}

func init() {
	rootCmd.AddCommand(newConfigCmd(), newConfigGlobalCmd())
}
