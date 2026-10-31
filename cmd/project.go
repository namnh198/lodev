package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"lodev/internal/certs"
	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/hosts"
	"lodev/internal/registry"
	"lodev/internal/templates"
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

func composeFile(p *config.Project) string    { return filepath.Join(p.Root, ".lodev", "compose.yaml") }
func composeProject(p *config.Project) string { return "lodev-" + p.Name }

func projectCompose(p *config.Project, sub ...string) error {
	return docker.Run(docker.ComposeArgs(composeProject(p), composeFile(p), sub...)...)
}

func mysqlExec(sql string) error {
	_, err := docker.Output("exec", "lodev-mysql", "mysql", "-uroot", "-proot", "-e", sql)
	return err
}

// newProjectConfig fills unset values from what is detected in root.
func newProjectConfig(root, name, typ, php, webserver string) (*config.Project, error) {
	if name == "" {
		name = config.SanitizeName(filepath.Base(root))
	}
	if typ == "" {
		typ = config.DetectType(root)
	}
	if php == "" {
		php = "8.3"
	}
	p := &config.Project{
		Name: name, Type: typ, PHP: php, Webserver: webserver,
		Docroot: config.DefaultDocroot(typ), Services: defaultServices[typ], Root: root,
	}
	p.ApplyDefaults()
	return p, p.Validate()
}

func ensureGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	for _, want := range []string{".lodev/compose.yaml", ".lodev/webserver.conf"} {
		if !contains(lines, want) {
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += want + "\n"
		}
	}
	if content == string(data) {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func contains(lines []string, s string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == s {
			return true
		}
	}
	return false
}

func start(p *config.Project, rebuild bool) error {
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
			fmt.Printf("hint: %s is disabled globally; started it for now. Run `lodev services enable %s` to keep it on.\n", svc, svc)
		}
	}
	if err := docker.EnsurePHPImage(p.PHP, rebuild); err != nil {
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
		conf, err := templates.WebserverConfig(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(p.Root, ".lodev", "webserver.conf"), conf, 0o644); err != nil {
			return err
		}
	}
	// WebDomains is nil for CLI projects, which also clears a block left from an earlier webserver.
	if err := hosts.Apply(p.Name, p.WebDomains()); err != nil {
		return err
	}
	reg.Put(p)
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
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(composeFile(p), data, 0o644); err != nil {
		return err
	}
	if err := projectCompose(p, "up", "-d"); err != nil {
		return err
	}
	fmt.Println()
	if p.Webserver == "none" {
		fmt.Println("CLI project ready: lodev ssh")
	}
	for _, d := range p.WebDomains() {
		fmt.Println("https://" + d)
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
	if _, err := os.Stat(composeFile(p)); err == nil {
		if err := projectCompose(p, "down"); err != nil {
			return err
		}
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
		fmt.Println("Dropped database", p.DBName())
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
	var name, typ, php, webserver string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create .lodev/config.yaml for the project in the current folder",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			if _, err := os.Stat(filepath.Join(root, ".lodev", "config.yaml")); err == nil {
				return errors.New("already initialized (.lodev/config.yaml exists)")
			}
			p, err := newProjectConfig(root, name, typ, php, webserver)
			if err != nil {
				return err
			}
			if err := config.SaveProject(p); err != nil {
				return err
			}
			if err := ensureGitignore(root); err != nil {
				return err
			}
			fmt.Printf("Created .lodev/config.yaml (%s, PHP %s, %s)\nRun: lodev start\n", p.Type, p.PHP, p.Webserver)
			return nil
		},
	}
	initCmd.Flags().StringVar(&name, "name", "", "project name (default: folder name)")
	initCmd.Flags().StringVar(&typ, "type", "", "laravel | wordpress | magento | php (default: detected)")
	initCmd.Flags().StringVar(&php, "php", "", "PHP version (default 8.3)")
	initCmd.Flags().StringVar(&webserver, "webserver", "", "nginx | apache | none (default nginx)")

	var rebuild bool
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the project (and the shared services if needed)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			p, err := currentProject()
			if err != nil {
				return err
			}
			return start(p, rebuild)
		},
	}
	startCmd.Flags().BoolVar(&rebuild, "rebuild", false, "rebuild the PHP image")

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
			if err := projectCompose(p, "stop"); err != nil {
				return err
			}
			return start(p, false)
		},
	}

	var dropDB, yes bool
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Remove the project container, hosts entries and certificate",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			p, err := currentProject()
			if err != nil {
				return err
			}
			return deleteProject(p, dropDB, yes)
		},
	}
	deleteCmd.Flags().BoolVar(&dropDB, "db", false, "also drop the project database")
	deleteCmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Args:  cobra.NoArgs,
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

	rootCmd.AddCommand(initCmd, startCmd, stopCmd, restartCmd, deleteCmd, listCmd)
}
