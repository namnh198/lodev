package cmd

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"lodev/internal/commands"
	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

// shippedCommands is the lowest layer: the command files embedded in lodev.
var shippedCommands = func() []commands.Command { return nil }

// hostRun runs a host command with the terminal attached; swapped in tests.
var hostRun = func(c *exec.Cmd) error {
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// loadScope reads dir/web and dir/host; a name in both keeps the web one.
func loadScope(dir string, keep func(commands.Command) bool) (map[string]commands.Command, []string) {
	web, warnings := commands.LoadDir(filepath.Join(dir, commands.Web), commands.Web)
	host, w := commands.LoadDir(filepath.Join(dir, commands.Host), commands.Host)
	warnings = append(warnings, w...)
	byName := map[string]commands.Command{}
	for _, c := range append(web, host...) {
		if !keep(c) {
			continue
		}
		if prev, ok := byName[c.Name]; ok {
			warnings = append(warnings, fmt.Sprintf("%s: %q is also in %s; using the web one", shortPath(c.Path), c.Name, shortPath(prev.Path)))
			continue
		}
		byName[c.Name] = c
	}
	return byName, warnings
}

// customCommands resolves the commands for wd. p is nil outside a project or when its config
// fails to load (projErr says why).
func customCommands(wd string, builtin map[string]bool) (cmds []commands.Command, p *config.Project, projErr error, warnings []string) {
	root, projErr := config.FindRoot(wd)
	if projErr == nil {
		if p, projErr = config.LoadProject(root); projErr != nil {
			p = nil
		}
	}
	applies := func(c commands.Command) bool { return p == nil || c.Applies(p.Type) }

	byName := map[string]commands.Command{}
	for _, c := range shippedCommands() {
		if applies(c) {
			byName[c.Name] = c
		}
	}
	// A marker file in ~/.lodev/commands is lodev's synced copy of a shipped one, not the user's.
	global, warnings := loadScope(filepath.Join(config.Home(), "commands"), func(c commands.Command) bool { return !c.Marker && applies(c) })
	for n, c := range global {
		byName[n] = c
	}
	if p != nil {
		dir := filepath.Join(p.Root, ".lodev", "commands")
		reg, _ := registry.Load()
		if registered(reg, p.Root) {
			project, w := loadScope(dir, applies)
			warnings = append(warnings, w...)
			for n, c := range project {
				byName[n] = c
			}
		} else {
			web, _ := commands.LoadDir(filepath.Join(dir, commands.Web), commands.Web)
			host, _ := commands.LoadDir(filepath.Join(dir, commands.Host), commands.Host)
			if len(web)+len(host) > 0 {
				warnings = append(warnings, ".lodev/commands is not loaded: run lodev create to review the project's commands")
			}
		}
	}

	for _, n := range slices.Sorted(maps.Keys(byName)) {
		c := byName[n]
		if builtin[n] {
			warnings = append(warnings, fmt.Sprintf("%s: %q is a lodev command; skipped", shortPath(c.Path), n))
			continue
		}
		cmds = append(cmds, c)
	}
	return cmds, p, projErr, warnings
}

// builtinNames is every name and alias a custom command may not take.
func builtinNames(root *cobra.Command) map[string]bool {
	names := map[string]bool{"help": true, "completion": true}
	for _, c := range root.Commands() {
		names[c.Name()] = true
		for _, a := range c.Aliases {
			names[a] = true
		}
	}
	return names
}

// registerCustom adds wd's custom commands to root, prints the warnings, and returns what it added.
func registerCustom(root *cobra.Command, wd string) []*cobra.Command {
	cmds, p, projErr, warnings := customCommands(wd, builtinNames(root))
	for _, w := range warnings {
		ui.Warn("%s", w)
	}
	var added []*cobra.Command
	for _, c := range cmds {
		cc := &cobra.Command{
			Use:                c.Use,
			Short:              c.Short,
			DisableFlagParsing: true,
			RunE: func(_ *cobra.Command, args []string) error {
				if c.Kind == commands.Web {
					return runWebCommand(c, p, projErr, args)
				}
				return runHostCommand(c, p, wd, args)
			},
		}
		root.AddCommand(cc)
		added = append(added, cc)
	}
	return added
}

func runWebCommand(c commands.Command, p *config.Project, projErr error, args []string) error {
	if p == nil {
		return projErr
	}
	if err := dockerPing(); err != nil {
		return err
	}
	return runInProject(p, "web", "", append([]string{"bash", "-c", c.Script, c.Name}, args...))
}

func runHostCommand(c commands.Command, p *config.Project, wd string, args []string) error {
	x := exec.Command("bash", append([]string{"-c", c.Script, c.Name}, args...)...)
	x.Dir = wd
	running := false
	if p != nil {
		x.Dir = p.Root
		running = containerRunning(p.WebContainer())
	}
	g, err := config.LoadGlobal()
	if err != nil {
		g, _ = config.ParseGlobal(nil)
	}
	bin, _ := executable()
	x.Env = append(os.Environ(), hostEnv(p, g, wd, bin, running)...)
	err = hostRun(x)
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		os.Exit(exitErr.ExitCode())
	case errors.Is(err, exec.ErrNotFound):
		return errors.New("host commands need bash on PATH")
	}
	return err
}

// hostEnv is the LODEV_* environment a host command gets on top of the user's.
func hostEnv(p *config.Project, g *config.Global, pwd, bin string, running bool) []string {
	env := []string{"LODEV_BIN=" + bin, "LODEV_PWD=" + pwd}
	if p == nil {
		return env
	}
	url := ""
	if p.Webserver != "none" && len(p.Domains) > 0 {
		url = "https://" + p.Domains[0]
	}
	user, _ := p.ContainerUser()
	status := "stopped"
	if running {
		status = "running"
	}
	env = append(env,
		"LODEV_PROJECT="+p.Name,
		"LODEV_ROOT="+p.Root,
		"LODEV_TYPE="+p.Type,
		"LODEV_URL="+url,
		"LODEV_WEB_CONTAINER="+p.WebContainer(),
		"LODEV_CONTAINER_USER="+user,
		"LODEV_WORKDIR="+p.WorkDir(),
		"LODEV_PROJECT_STATUS="+status,
	)
	if db := p.DB(); db != "" {
		port := g.Ports.MySQL
		if db == "mariadb" {
			port = g.Ports.MariaDB
		}
		env = append(env,
			"LODEV_DB_TYPE="+db,
			"LODEV_DB_HOST=127.0.0.1",
			"LODEV_DB_PORT="+port,
			"LODEV_DB_USER=root",
			"LODEV_DB_PASSWORD=root",
			"LODEV_DB_NAME="+p.DBName(),
		)
	}
	return env
}
