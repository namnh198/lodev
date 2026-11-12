package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/registry"
)

// serviceHosts maps lodev open's service targets to their host name on the TLD.
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

// openURL opens url in the default browser; swapped in tests.
var openURL = func(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
	}
	return exec.Command("xdg-open", url).Run()
}

func newOpenCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "open [project|phpmyadmin|mailpit|rabbitmq|mongo-express|traefik]",
		Short: "Open a project's or a shared service's page in the browser",
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
			if print, _ := c.Flags().GetBool("print"); print {
				fmt.Fprintln(c.OutOrStdout(), url)
				return nil
			}
			if err := openURL(url); err != nil {
				fmt.Fprintf(c.OutOrStdout(), "%s\n(could not open a browser: %v)\n", url, err)
			}
			return nil
		},
	}
	c.Flags().Bool("print", false, "print the URL instead of opening it")
	return c
}

func init() { rootCmd.AddCommand(newOpenCmd()) }
