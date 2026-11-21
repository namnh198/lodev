package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"lodev/internal/commands"
	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

// completeTimeout caps an autocomplete script, so a TAB never hangs.
var completeTimeout = 5 * time.Second

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

// registerCustom adds wd's custom commands to root, prints the warnings unless quiet (a
// completion request, whose output the shell reads), and returns what it added.
func registerCustom(root *cobra.Command, wd string, quiet bool) []*cobra.Command {
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
		if !quiet {
			ui.Warn("%s", w)
		}
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
		if c.Complete != nil {
			cc.ValidArgsFunction = func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return completeCustom(c, p, wd, args, toComplete)
			}
		}
		root.AddCommand(cc)
		added = append(added, cc)
	}
	return added
}

// completeCustom runs c's autocomplete script with the words typed so far, the last being the
// word being completed; anything but suggestions lets the shell fall back to files.
func completeCustom(c commands.Command, p *config.Project, wd string, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ac, argv := *c.Complete, append(slices.Clone(args), toComplete)
	if ac.Kind == commands.Web && (p == nil || !containerRunning(p.WebContainer())) {
		return nil, cobra.ShellCompDirectiveDefault
	}
	ctx, cancel := context.WithTimeout(context.Background(), completeTimeout)
	defer cancel()
	done := make(chan string, 1)
	go func() {
		var out string
		if ac.Kind == commands.Web {
			user, _ := p.ContainerUser()
			out, _ = dockerOutput(docker.ExecArgs(p.ComposeProject(), composeFile(p), "web", user, p.WorkDir(), false, webArgv(ac, argv))...)
		} else if x, cleanup, err := hostProcess(ctx, ac, p, wd, argv); err == nil {
			defer cleanup()
			b, _ := x.Output() // stdin and stderr stay nil: /dev/null
			out = string(b)
		}
		done <- out
	}()
	select {
	case out := <-done:
		if s := commands.FilterSuggestions(out, toComplete); len(s) > 0 {
			return s, cobra.ShellCompDirectiveNoFileComp
		}
	case <-ctx.Done():
	}
	return nil, cobra.ShellCompDirectiveDefault
}

// runFile runs $1 (a script) as an executable, so its shebang picks the interpreter; $0 is the temp file.
const runFile = `f=$(mktemp) || exit 1; printf '%s' "$1" > "$f" && chmod 700 "$f" || exit 1; shift; "$f" "$@"; s=$?; rm -f "$f"; exit $s`

// webArgv is the command that runs c's script with args in the web container.
func webArgv(c commands.Command, args []string) []string {
	if c.Interp {
		return append([]string{"sh", "-c", runFile, c.Name, c.Script}, args...)
	}
	return append([]string{"bash", "-c", c.Script, c.Name}, args...)
}

func runWebCommand(c commands.Command, p *config.Project, projErr error, args []string) error {
	if p == nil {
		return projErr
	}
	if err := dockerPing(); err != nil {
		return err
	}
	return runInProject(p, "web", "", webArgv(c, args))
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
	x, cleanup, err := hostProcess(context.Background(), c, p, wd, args)
	if err != nil {
		return err
	}
	defer cleanup()
	return hostRun(x)
}

// hostProcess builds the process that runs c's script with args on this machine; cleanup
// removes the interpreter temp file once it has finished.
func hostProcess(ctx context.Context, c commands.Command, p *config.Project, wd string, args []string) (*exec.Cmd, func(), error) {
	x, cleanup := exec.CommandContext(ctx, "bash", append([]string{"-c", c.Script, c.Name}, args...)...), func() {}
	if c.Interp {
		f, err := os.CreateTemp("", "lodev-"+c.Name+"-*")
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() { os.Remove(f.Name()) }
		_, werr := f.WriteString(c.Script)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Chmod(f.Name(), 0o700)
		}
		if werr != nil {
			cleanup()
			return nil, nil, werr
		}
		x = exec.CommandContext(ctx, f.Name(), args...)
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
	return x, cleanup, nil
}
