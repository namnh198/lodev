package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/registry"
)

// serviceHosts maps lodev url's service targets to their host name on the TLD.
var serviceHosts = map[string]string{"phpmyadmin": "phpmyadmin", "mailpit": "mailpit", "rabbitmq": "rabbitmq", "mongo-express": "mongo"}

// openTarget is the URL for target: a registered project, a shared service's page, traefik's
// dashboard, or ("" ) the project here. A project wins over a service of the same name.
func openTarget(target string, reg registry.Registry, g *config.Global, here *config.Project) (string, error) {
	p := here
	if target != "" {
		e, ok := reg[target]
		switch {
		case ok:
			lp, err := config.LoadProject(e.Path)
			if err != nil {
				return "", err
			}
			p = lp
		case target == "traefik":
			return "http://localhost:" + strconv.Itoa(g.Ports.Dashboard), nil
		case serviceHosts[target] != "":
			if !g.Services[target] {
				return "", fmt.Errorf("%s is disabled (lodev service enable %s)", target, target)
			}
			return "https://" + serviceHosts[target] + "." + g.TLD, nil
		default:
			return "", fmt.Errorf("unknown target %q (a project from lodev list, or phpmyadmin, mailpit, rabbitmq, mongo-express, traefik)", target)
		}
	}
	if p == nil {
		return "", fmt.Errorf("not in a lodev project: give a project or a service")
	}
	if p.Webserver == "none" || len(p.Domains) == 0 {
		return "", fmt.Errorf("project %s has no web URL (webserver: none)", p.Name)
	}
	return "https://" + p.Domains[0], nil
}

func newURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url [project|phpmyadmin|mailpit|rabbitmq|mongo-express|traefik]",
		Short: "Print a project's or a shared service's URL",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			reg, err := registry.Load()
			if err != nil {
				return err
			}
			g, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			here, _ := currentProject() // nil outside a project
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			url, err := openTarget(target, reg, g, here)
			if err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), url)
			return nil
		},
	}
}

func init() { rootCmd.AddCommand(newURLCmd()) }
