package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
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
	case "n98":
		return []string{"n98-magerun2"}
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
	return runInProject(p, service, user, cmd)
}

// runInProject runs cmd in service of p (web: as the project user, in its working_dir).
func runInProject(p *config.Project, service, user string, cmd []string) error {
	err := execIn(p, service, user, cmd)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}

// execInWeb runs cmd in p's web container like `lodev exec`, returning its exit status as an error.
func execInWeb(p *config.Project, cmd []string) error { return execIn(p, "web", "", cmd) }

func execIn(p *config.Project, service, user string, cmd []string) error {
	workdir := ""
	if service == "web" {
		workdir = p.WorkDir()
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
	return dockerRun(docker.ExecArgs(composeProject(p), composeFile(p), service, user, workdir, stdinTTY(), cmd)...)
}

// showLogs follows a service's logs: shared services (and traefik) by container, others via the project's compose.
// showLogsJSON follows the web logs through jq in the web container: JSON lines come out
// pretty, other lines as they are.
func showLogsJSON(service string) error {
	if service != "web" {
		return errors.New("--json works only with the web service")
	}
	p, err := currentProject()
	if err != nil {
		return err
	}
	return dockerPipe(docker.ComposeArgs(composeProject(p), composeFile(p), "logs", "-f", "--no-log-prefix", "web"),
		[]string{"exec", "-i", "lodev-" + p.Name + "-web", "jq", "-R", "-r", "--unbuffered", "fromjson? // ."})
}

func showLogs(service string) error {
	if service == "" {
		return errServiceName
	}
	if service == "traefik" || slices.Contains(config.SharedServices, service) {
		return dockerRun("logs", "-f", "lodev-"+service)
	}
	p, err := currentProject()
	if err != nil {
		return err
	}
	return dockerRun(docker.ComposeArgs(composeProject(p), composeFile(p), "logs", "-f", service)...)
}

// checkWebUser fails early for containers started by an older lodev, which lack the host user.
func checkWebUser(p *config.Project, user string) error {
	if user == "" || user == "root" || userReady(p, user) {
		return nil
	}
	ui.Info("hint: this container predates the current lodev; run lodev restart")
	return fmt.Errorf("user %q does not exist in the web container", user)
}

const reloadFPM = ` && { [ "$LODEV_WEBSERVER" = none ] || supervisorctl -c /etc/supervisor/supervisord.conf restart php-fpm >/dev/null; } && echo "xdebug `

// xdebugScript returns the root shell script for a `lodev xdebug` action, or "" if unknown.
func xdebugScript(action string) string {
	on := `for s in cli fpm; do cp /etc/lodev/xdebug.ini /etc/php/$LODEV_PHP/$s/conf.d/99-lodev-xdebug.ini; done` + reloadFPM + `on"`
	off := `rm -f /etc/php/$LODEV_PHP/*/conf.d/99-lodev-xdebug.ini` + reloadFPM + `off"`
	return map[string]string{
		"on":     on,
		"off":    off,
		"toggle": "if php -m | grep -qi xdebug; then " + off + "; else " + on + "; fi",
		"status": "php -m | grep -qi xdebug && echo on || echo off",
	}[action]
}

func init() {
	var sshService string
	sshCmd := &cobra.Command{
		Use:   "ssh [name]",
		Short: "Open a shell in a service container (default web) of the project here or [name]",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if sshService == "" {
				return errServiceName
			}
			p, err := projectArg(args)
			if err != nil {
				return err
			}
			shell := []string{"bash"}
			if sshService != "web" {
				shell = []string{"sh", "-c", "command -v bash >/dev/null && exec bash || exec sh"}
			}
			return runInProject(p, sshService, "", shell)
		},
	}
	sshCmd.Flags().StringVarP(&sshService, "service", "s", "web", "service to open a shell in")

	var logsService string
	var logsJSON bool
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Follow the logs of a service (default web; shared services like mysql too)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if logsJSON {
				return showLogsJSON(logsService)
			}
			return showLogs(logsService)
		},
	}
	logsCmd.Flags().StringVarP(&logsService, "service", "s", "web", "service to show the logs of")
	logsCmd.Flags().BoolVarP(&logsJSON, "json", "j", false, "pretty-print JSON log lines with jq (web only)")

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
	rootCmd.AddCommand(sshCmd, execCmd, logsCmd)

	for _, name := range []string{"php", "composer", "npm", "node", "artisan", "wp", "magento", "n98"} {
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
		Use:       "xdebug on|off|toggle|status",
		Short:     "Toggle Xdebug in the web container",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"on", "off", "toggle", "status"},
		RunE: func(_ *cobra.Command, args []string) error {
			if xdebugScript(args[0]) == "" {
				return errors.New("usage: lodev xdebug on|off|toggle|status")
			}
			if args[0] == "status" {
				return runInService("web", "root", []string{"sh", "-c", xdebugScript("status")})
			}
			p, err := currentProject()
			if err != nil {
				return err
			}
			return setXdebug(p, xdebugTarget(args[0], p))
		},
	}
	rootCmd.AddCommand(xdebugCmd)
}

// xdebugTarget is whether action (on, off, toggle) leaves Xdebug on; toggle flips enable_xdebug.
func xdebugTarget(action string, p *config.Project) bool {
	if action == "toggle" {
		return !p.EnableXdebug
	}
	return action == "on"
}

// setXdebug switches Xdebug in the running web container, then records it as enable_xdebug in
// .lodev/config.local.yaml so the next start keeps it; a stopped project only gets the setting.
func setXdebug(p *config.Project, on bool) error {
	state := map[bool]string{true: "on", false: "off"}[on]
	if containerRunning("lodev-" + p.Name + "-web") {
		if err := execIn(p, "web", "root", []string{"sh", "-c", xdebugScript(state)}); err != nil {
			return err
		}
	} else {
		ui.Info("xdebug %s: applies on the next lodev start", state)
	}
	_, err := setLocal(p.Root, "enable_xdebug", on)
	return err
}

// syncXdebug makes a started web container match enable_xdebug: on when enabled, off only when a
// kept (not recreated) container still has it, so a plain start does not restart php-fpm.
func syncXdebug(p *config.Project) {
	state, script := "on", xdebugScript("on")
	if !p.EnableXdebug {
		state, script = "off", "[ ! -e /etc/php/$LODEV_PHP/cli/conf.d/99-lodev-xdebug.ini ] || { "+xdebugScript("off")+"; }"
	}
	if err := execIn(p, "web", "root", []string{"sh", "-c", script}); err != nil {
		ui.Warn("could not switch Xdebug %s: %v (run lodev xdebug %s)", state, err, state)
	}
}
