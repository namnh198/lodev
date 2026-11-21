package cmd

import (
	"errors"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"lodev/internal/commands"
	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

// shippedCommands is the lowest layer: the command files embedded in lodev.
var shippedCommands = commands.Shipped

// hostRun runs a host command with the terminal attached; swapped in tests.
var hostRun = func(c *exec.Cmd) error {
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
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
	var p *config.Project
	dir, projErr := config.FindRoot(wd)
	if projErr == nil {
		if p, projErr = config.LoadProject(dir); projErr != nil {
			p = nil
		}
	}
	projectRoot, projectType, isRegistered := "", "", false
	if p != nil {
		reg, _ := registry.Load()
		projectRoot, projectType, isRegistered = p.Root, p.Type, registered(reg, p.Root)
	}
	cmds, warnings := commands.Resolve(shippedCommands(), config.Home(), projectRoot, projectType, isRegistered, builtinNames(root))
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

// runFile runs $1 (a script) as an executable, so its shebang picks the interpreter; $0 is the temp file.
const runFile = `f=$(mktemp) || exit 1; printf '%s' "$1" > "$f" && chmod 700 "$f" || exit 1; shift; "$f" "$@"; s=$?; rm -f "$f"; exit $s`

func runWebCommand(c commands.Command, p *config.Project, projErr error, args []string) error {
	if p == nil {
		return projErr
	}
	if err := dockerPing(); err != nil {
		return err
	}
	if c.Interp {
		return runInProject(p, "web", "", append([]string{"sh", "-c", runFile, c.Name, c.Script}, args...))
	}
	return runInProject(p, "web", "", append([]string{"bash", "-c", c.Script, c.Name}, args...))
}

func runHostCommand(c commands.Command, p *config.Project, wd string, args []string) error {
	err := hostCommand(c, p, wd, args)
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		os.Exit(exitErr.ExitCode())
	case errors.Is(err, exec.ErrNotFound):
		return errors.New("host commands need bash on PATH")
	}
	return err
}

func hostCommand(c commands.Command, p *config.Project, wd string, args []string) error {
	x := exec.Command("bash", append([]string{"-c", c.Script, c.Name}, args...)...)
	if c.Interp {
		f, err := os.CreateTemp("", "lodev-"+c.Name+"-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		_, werr := f.WriteString(c.Script)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Chmod(f.Name(), 0o700)
		}
		if werr != nil {
			return werr
		}
		x = exec.Command(f.Name(), args...)
	}
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
	x.Env = append(commands.DropEnv(os.Environ()), commands.HostEnv(p, g, wd, bin, running)...)
	return hostRun(x)
}
