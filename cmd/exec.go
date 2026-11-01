package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/ui"
)

func shortcutCmd(name string) []string {
	switch name {
	case "artisan":
		return []string{"php", "artisan"}
	case "magento":
		return []string{"php", "bin/magento"}
	}
	return []string{name}
}

var errServiceName = errors.New("-s/--service needs a service name")

// splitService consumes one leading -s/--service flag; the rest is the command, unchanged.
func splitService(args []string) (svc string, rest []string, err error) {
	if len(args) == 0 {
		return "web", args, nil
	}
	switch a := args[0]; {
	case a == "-s" || a == "--service":
		if len(args) < 2 || args[1] == "" {
			return "", nil, errServiceName
		}
		return args[1], args[2:], nil
	case strings.HasPrefix(a, "--service="):
		if svc = strings.TrimPrefix(a, "--service="); svc == "" {
			return "", nil, errServiceName
		}
		return svc, args[1:], nil
	}
	return "web", args, nil
}

// runInService runs cmd in a project service; its exit code becomes lodev's.
// Only web gets the container user (when user is empty) and the project workdir.
func runInService(service, user string, cmd []string) error {
	p, err := currentProject()
	if err != nil {
		return err
	}
	workdir := ""
	if service == "web" {
		workdir = "/var/www/html"
		if user == "" {
			user, _ = p.ContainerUser()
		}
	} else {
		user = ""
	}
	if service == "web" {
		if err := checkWebUser(p, user); err != nil {
			return err
		}
	}
	err = docker.Run(docker.ExecArgs(composeProject(p), composeFile(p), service, user, workdir, stdinTTY(), cmd)...)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}

// checkWebUser fails early for containers started by an older lodev, which lack the host user.
func checkWebUser(p *config.Project, user string) error {
	if user == "" || user == "root" || userReady(p, user) {
		return nil
	}
	ui.Info("hint: this container predates the current lodev; run lodev restart")
	return fmt.Errorf("user %q does not exist in the web container", user)
}

const reloadFPM = ` && { [ "$LODEV_WEBSERVER" = none ] || supervisorctl restart php-fpm >/dev/null; } && echo "xdebug `

func init() {
	var sshService string
	sshCmd := &cobra.Command{
		Use:   "ssh",
		Short: "Open a shell in a service container (default web)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if sshService == "" {
				return errServiceName
			}
			shell := []string{"bash"}
			if sshService != "web" {
				shell = []string{"sh", "-c", "command -v bash >/dev/null && exec bash || exec sh"}
			}
			return runInService(sshService, "", shell)
		},
	}
	sshCmd.Flags().StringVarP(&sshService, "service", "s", "web", "service to open a shell in")

	execCmd := &cobra.Command{
		Use:                "exec [-s service] <command...>",
		Short:              "Run a command in a service container (default web)",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			svc, rest, err := splitService(args)
			if err != nil {
				return err
			}
			if len(rest) == 0 {
				return errors.New("usage: lodev exec [-s service] <command...>")
			}
			return runInService(svc, "", rest)
		},
	}
	rootCmd.AddCommand(sshCmd, execCmd)

	for _, name := range []string{"php", "composer", "npm", "node", "artisan", "wp", "magento"} {
		name := name
		rootCmd.AddCommand(&cobra.Command{
			Use:                name + " [args...]",
			Short:              "Run " + name + " in the web container",
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error {
				return runInService("web", "", append(shortcutCmd(name), args...))
			},
		})
	}

	xdebugCmd := &cobra.Command{
		Use:       "xdebug on|off|status",
		Short:     "Toggle Xdebug in the web container",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"on", "off", "status"},
		RunE: func(_ *cobra.Command, args []string) error {
			script := map[string]string{
				"on":     `for s in cli fpm; do cp /etc/lodev/xdebug.ini /etc/php/$LODEV_PHP/$s/conf.d/99-lodev-xdebug.ini; done` + reloadFPM + `on"`,
				"off":    `rm -f /etc/php/$LODEV_PHP/*/conf.d/99-lodev-xdebug.ini` + reloadFPM + `off"`,
				"status": "php -m | grep -qi xdebug && echo on || echo off",
			}[args[0]]
			if script == "" {
				return errors.New("usage: lodev xdebug on|off|status")
			}
			return runInService("web", "root", []string{"sh", "-c", script})
		},
	}
	rootCmd.AddCommand(xdebugCmd)
}
