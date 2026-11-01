package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"lodev/internal/certs"
	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/hosts"
	"lodev/internal/registry"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

var defaultServices = map[string][]string{
	"magento":   {"mysql", "redis", "opensearch"},
	"laravel":   {"mysql", "redis"},
	"wordpress": {"mysql"},
	"php":       {"mysql"},
}

func currentProject() (*config.Project, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, err := config.FindRoot(wd)
	if err != nil {
		return nil, err
	}
	return config.LoadProject(root)
}

// WebserverConfFile is the generated webserver config, mounted into the container.
func WebserverConfFile(p *config.Project) string {
	return filepath.Join(p.Root, ".lodev", p.Webserver+"-webserver.conf")
}

func composeBase(p *config.Project) string {
	return filepath.Join(p.Root, ".lodev", "lodev.compose-base.yml")
}

func composeFull(p *config.Project) string {
	return filepath.Join(p.Root, ".lodev", "lodev.compose-full.yml")
}

// composeFile is the merged file every compose call uses.
// Projects last started by an older lodev only have the legacy file.
func composeFile(p *config.Project) string {
	full := composeFull(p)
	if _, err := os.Stat(full); err != nil {
		if _, lerr := os.Stat(composeLegacy(p)); lerr == nil {
			return composeLegacy(p)
		}
	}
	return full
}
func composeLegacy(p *config.Project) string  { return filepath.Join(p.Root, ".lodev", "compose.yaml") }
func composeProject(p *config.Project) string { return "lodev-" + p.Name }

var composeConfig = func(args ...string) (string, error) { return docker.Output(args...) }

// extraComposeFiles lists the user's .lodev/*.compose.{yml,yaml}, sorted.
func extraComposeFiles(p *config.Project) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(p.Root, ".lodev"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var files []string
	for _, e := range entries { // ReadDir is sorted by name
		n := e.Name()
		if e.Type().IsRegular() && (strings.HasSuffix(n, ".compose.yml") || strings.HasSuffix(n, ".compose.yaml")) {
			files = append(files, filepath.Join(p.Root, ".lodev", n))
		}
	}
	return files, nil
}

// mergeCompose writes the full file only if `docker compose config` succeeds.
func mergeCompose(p *config.Project) error {
	extra, err := extraComposeFiles(p)
	if err != nil {
		return err
	}
	out, err := composeConfig(docker.ComposeConfigArgs(composeProject(p), append([]string{composeBase(p)}, extra...))...)
	if err != nil {
		return err
	}
	return os.WriteFile(composeFull(p), []byte(out+"\n"), 0o644)
}

var dockerOutput = docker.Output

// removeProjectImage drops the built web image; a project that never built one has none.
func removeProjectImage(name string) error {
	if _, err := dockerOutput("rmi", config.ProjectImage(name)); err != nil && !strings.Contains(err.Error(), "No such image") {
		return err
	}
	return nil
}

// dropStaleProjectImage removes a no-longer-needed project image once compose
// runs the container on img; it is a no-op while img is still the project image.
func dropStaleProjectImage(p *config.Project, img string) error {
	if img == config.ProjectImage(p.Name) {
		return nil
	}
	return removeProjectImage(p.Name)
}

func userWarning(name, from string) string {
	if from == "" {
		return ""
	}
	return fmt.Sprintf("%q is not a valid container user name; using %q (set user: in .lodev/config.yaml to choose another)", from, name)
}

var userPoll = time.Second

// userReady reports whether the entrypoint has created user in the web container.
var userReady = func(p *config.Project, user string) bool {
	_, err := docker.Output(docker.ExecArgs(composeProject(p), composeFile(p), "web", "", "", false, []string{"id", "-u", user})...)
	return err == nil
}

// waitForUser blocks until the entrypoint has created user, so ssh/exec right after start work.
func waitForUser(p *config.Project, user string, timeout time.Duration) error {
	for deadline := time.Now().Add(timeout); ; time.Sleep(userPoll) {
		if userReady(p, user) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("user %q was not created in the web container; check: docker logs lodev-%s-web", user, p.Name)
		}
	}
}

func projectCompose(p *config.Project, sub ...string) error {
	return docker.Run(docker.ComposeArgs(composeProject(p), composeFile(p), sub...)...)
}

func mysqlExec(sql string) error {
	_, err := docker.Output("exec", "lodev-mysql", "mysql", "-uroot", "-proot", "-e", sql)
	return err
}

// newProjectConfig fills unset values from what is detected in root.
func newProjectConfig(root, name, typ, php, webserver, docroot string) (*config.Project, error) {
	if name == "" {
		if name = config.SanitizeName(filepath.Base(root)); name == "" {
			return nil, fmt.Errorf("cannot derive a project name from folder %q; use --name", filepath.Base(root))
		}
	}
	if typ == "" {
		typ = config.DetectType(root)
	}
	if php == "" {
		php = "8.3"
	}
	switch docroot {
	case "":
		docroot = config.DefaultDocroot(typ)
	case ".":
		docroot = ""
	}
	p := &config.Project{
		Name: name, Type: typ, PHP: php, Webserver: webserver,
		Docroot: docroot, Services: defaultServices[typ], Root: root,
	}
	p.ApplyDefaults()
	return p, p.Validate()
}

// generated reports whether path is missing or still carries the lodev marker as its first line.
func generated(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	first, _, _ := strings.Cut(string(b), "\n")
	return first == templates.GeneratedMarker
}

func writeWebserverConf(p *config.Project) error {
	if p.Webserver == "none" {
		return nil
	}
	path := WebserverConfFile(p)
	if !generated(path) {
		ui.Info(".lodev/%s-webserver.conf is user-managed; not regenerating", p.Webserver)
		return nil
	}
	conf, err := templates.WebserverConfig(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, conf, 0o644)
}

// writeLodevGitignore ignores generated files; a webserver conf is listed only while lodev owns it.
func writeLodevGitignore(p *config.Project) error {
	dir := filepath.Join(p.Root, ".lodev")
	s := "# Generated by lodev on every `lodev start`; do not edit.\n/lodev.compose-base.yml\n/lodev.compose-full.yml\n/web-build/\n" +
		"# each <ws>-webserver.conf below is listed only while it carries the marker\n"
	for _, ws := range []string{"nginx", "apache"} {
		if generated(filepath.Join(dir, ws+"-webserver.conf")) {
			s += "/" + ws + "-webserver.conf\n"
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(s), 0o644)
}

func start(p *config.Project, rebuild bool) error {
	user, from := p.ContainerUser()
	if w := userWarning(user, from); w != "" {
		ui.Warn("%s", w)
	}
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	if err := reg.Conflict(p); err != nil {
		return err
	}
	if err := servicesUp(); err != nil {
		return err
	}
	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	for _, svc := range p.Services {
		if !g.Services[svc] {
			if err := startService(svc); err != nil {
				return err
			}
			ui.Info("hint: %s is disabled globally; started it for now. Run `lodev services enable %s` to keep it on.", svc, svc)
		}
	}
	if err := docker.EnsurePHPImage(p.PHP, rebuild); err != nil {
		return err
	}
	img, err := docker.EnsureProjectImage(p, rebuild)
	if err != nil {
		return err
	}
	if p.Uses("mysql") {
		if err := docker.WaitHealthy("lodev-mysql", 60*time.Second); err != nil {
			return err
		}
		if err := mysqlExec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", p.DBName())); err != nil {
			return err
		}
	}
	if p.Webserver != "none" {
		if err := certs.Ensure(p.Name, p.Domains); err != nil {
			return err
		}
		if err := certs.SyncTraefik(); err != nil {
			return err
		}
		if err := writeWebserverConf(p); err != nil {
			return err
		}
		legacy := filepath.Join(p.Root, ".lodev", "webserver.conf")
		if err := os.Remove(legacy); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	// Also creates the bind-mounted php/ and <webserver>/ dirs, and brings projects made before them up to date.
	if err := scaffold(p.Root, p.Webserver); err != nil {
		return err
	}
	if err := writeLodevGitignore(p); err != nil {
		return err
	}
	// WebDomains is nil for CLI projects, which also clears a block left from an earlier webserver.
	if err := hosts.Apply(p.Name, p.WebDomains()); err != nil {
		return err
	}
	dropped := reg.Put(p)
	for _, old := range dropped {
		// Renamed project: retire the old name's container, hosts block and cert (its database is kept).
		ui.Info("Project was renamed from %s; removing its old container, hosts entries and certificate.", old)
		if err := docker.Run("compose", "-p", "lodev-"+old, "down"); err != nil {
			return err
		}
		if err := removeProjectImage(old); err != nil {
			return err
		}
		if err := hosts.Apply(old, nil); err != nil {
			return err
		}
		if err := certs.Remove(old); err != nil {
			return err
		}
	}
	if len(dropped) > 0 {
		if err := certs.SyncTraefik(); err != nil {
			return err
		}
	}
	if err := reg.Save(); err != nil {
		return err
	}
	ca, err := certs.CAFile()
	if err != nil {
		return err
	}
	data, err := templates.ProjectCompose(templates.ProjectInput{
		Project:    p,
		ExtraHosts: append(reg.AllDomains(), config.ReservedDomains...),
		CAFile:     ca,
		Image:      img,
		User:       user,
		UID:        os.Getuid(),
		GID:        os.Getgid(),
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(composeBase(p), data, 0o644); err != nil {
		return err
	}
	if err := mergeCompose(p); err != nil {
		return err
	}
	if err := os.Remove(composeLegacy(p)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := projectCompose(p, "up", "-d", "--remove-orphans"); err != nil {
		return err
	}
	if err := waitForUser(p, user, 10*time.Second); err != nil {
		return err
	}
	if err := dropStaleProjectImage(p, img); err != nil {
		ui.Warn("could not remove the old project image: %v", err)
	}
	fmt.Println()
	if p.Webserver == "none" {
		ui.Success("CLI project ready: lodev ssh")
	}
	for _, d := range p.WebDomains() {
		ui.Success("https://%s", d)
	}
	return nil
}

func deleteProject(p *config.Project, dropDB, yes bool) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}
	// A copy (clone, worktree, cp -R) shares the name; never tear down the original.
	if err := reg.Conflict(p); err != nil {
		return fmt.Errorf("refusing to delete: %w", err)
	}
	// A project never started with the new lodev only has the legacy file.
	if _, err := os.Stat(composeFile(p)); err == nil {
		if err := projectCompose(p, "down"); err != nil {
			return err
		}
	}
	if err := removeProjectImage(p.Name); err != nil {
		return err
	}
	if err := hosts.Apply(p.Name, nil); err != nil {
		return err
	}
	if err := certs.Remove(p.Name); err != nil {
		return err
	}
	if err := certs.SyncTraefik(); err != nil {
		return err
	}
	delete(reg, p.Name)
	if err := reg.Save(); err != nil {
		return err
	}
	if dropDB && confirm(fmt.Sprintf("Drop database %s?", p.DBName()), yes) {
		if err := mysqlExec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", p.DBName())); err != nil {
			return err
		}
		ui.Success("Dropped database %s", p.DBName())
	}
	return nil
}

func status(name string) string {
	out, _ := docker.Output("ps", "-a", "--filter", "name=^lodev-"+name+"-web$", "--format", "{{.State}}")
	if out == "" {
		return "stopped"
	}
	return out
}

func init() {
	var name, typ, php, webserver, docroot string
	var noAsk bool
	createCmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"init"},
		Short:   "Create .lodev/config.yaml for the project in the current folder",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			if _, err := os.Stat(filepath.Join(root, ".lodev", "config.yaml")); err == nil {
				return errors.New("already initialized (.lodev/config.yaml exists)")
			}
			p, err := newProjectConfig(root, name, typ, php, webserver, docroot)
			if err != nil {
				return err
			}
			if !noAsk && stdinTTY() {
				set := map[string]bool{}
				c.Flags().Visit(func(f *pflag.Flag) { set[f.Name] = true })
				if err := askProject(p, set); err != nil {
					return err
				}
			}
			if err := config.SaveProject(p); err != nil {
				return err
			}
			if err := scaffold(root, p.Webserver); err != nil {
				return err
			}
			if err := writeLodevGitignore(p); err != nil {
				return err
			}
			ui.Success("Created .lodev/config.yaml (%s, PHP %s, %s)\nRun: lodev start", p.Type, p.PHP, p.Webserver)
			return nil
		},
	}
	createCmd.Flags().StringVar(&name, "name", "", "project name (default: folder name)")
	createCmd.Flags().StringVar(&typ, "type", "", "laravel | wordpress | magento | php (default: detected)")
	createCmd.Flags().StringVar(&php, "php", "", "PHP version (default 8.3)")
	createCmd.Flags().StringVar(&webserver, "webserver", "", "nginx | apache | none (default nginx)")
	createCmd.Flags().StringVar(&docroot, "docroot", "", `document root relative to the project; "." for the project root (default: by type)`)
	createCmd.Flags().BoolVar(&noAsk, "no-ask", false, "do not prompt; use flags and detected values")

	var rebuild bool
	startCmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"up"},
		Short:   "Start the project (and the shared services if needed)",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			p, err := currentProject()
			if err != nil {
				return err
			}
			return withLock(func() error { return start(p, rebuild) })
		},
	}
	startCmd.Flags().BoolVar(&rebuild, "rebuild", false, "rebuild the PHP image and the project image")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the project container",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			p, err := currentProject()
			if err != nil {
				return err
			}
			return projectCompose(p, "stop")
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Stop and start the project",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			p, err := currentProject()
			if err != nil {
				return err
			}
			return withLock(func() error {
				if err := projectCompose(p, "stop"); err != nil {
					return err
				}
				return start(p, false)
			})
		},
	}

	var dropDB, yes bool
	removeCmd := &cobra.Command{
		Use:     "remove",
		Aliases: []string{"down"},
		Short:   "Remove the project container, hosts entries and certificate",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			p, err := currentProject()
			if err != nil {
				return err
			}
			return withLock(func() error { return deleteProject(p, dropDB, yes) })
		},
	}
	removeCmd.Flags().BoolVar(&dropDB, "db", false, "also drop the project database")
	removeCmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ps"},
		Short:   "List projects",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			reg, err := registry.Load()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS\tTYPE\tDOMAINS\tPATH")
			for _, name := range reg.Names() {
				e := reg[name]
				typ := "missing"
				if p, err := config.LoadProject(e.Path); err == nil {
					typ = p.Type
				}
				domains := strings.Join(e.Domains, ",")
				if domains == "" {
					domains = "CLI"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, status(name), typ, domains, e.Path)
			}
			return w.Flush()
		},
	}

	rootCmd.AddCommand(createCmd, startCmd, stopCmd, restartCmd, removeCmd, listCmd)
}
