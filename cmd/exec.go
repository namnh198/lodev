package cmd

import (
	"errors"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"lodev/internal/docker"
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

// runInWeb runs cmd in the project's web container; its exit code becomes lodev's.
func runInWeb(user string, cmd []string) error {
	p, err := currentProject()
	if err != nil {
		return err
	}
	fi, err := os.Stdin.Stat()
	tty := err == nil && fi.Mode()&os.ModeCharDevice != 0
	err = docker.Run(docker.ExecArgs(composeProject(p), composeFile(p), user, tty, cmd)...)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}

const reloadFPM = ` && { [ "$LODEV_WEBSERVER" = none ] || supervisorctl restart php-fpm >/dev/null; } && echo "xdebug `

func init() {
	sshCmd := &cobra.Command{
		Use:   "ssh",
		Short: "Open a shell in the web container",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return runInWeb("www-data", []string{"bash"}) },
	}

	execCmd := &cobra.Command{
		Use:                "exec <command...>",
		Short:              "Run a command in the web container",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE:               func(_ *cobra.Command, args []string) error { return runInWeb("www-data", args) },
	}
	rootCmd.AddCommand(sshCmd, execCmd)

	for _, name := range []string{"composer", "npm", "node", "artisan", "wp", "magento"} {
		name := name
		rootCmd.AddCommand(&cobra.Command{
			Use:                name + " [args...]",
			Short:              "Run " + name + " in the web container",
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error {
				return runInWeb("www-data", append(shortcutCmd(name), args...))
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
			return runInWeb("root", []string{"sh", "-c", script})
		},
	}
	rootCmd.AddCommand(xdebugCmd)
}
