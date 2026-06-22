package cmd

import (
	"errors"
	"os"
	"strings"

	"github.com/docker/cli/cli"
	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
)

// execDirArg allows a configurable container execution directory
var execDirArg string

var ExecCmd = &cobra.Command{
	Use:     "exec [flags] [command] [command-flags]",
	Aliases: []string{"."},
	Short:   "Execute a shell command in the container for a service. Uses the web service by default.",
	Long:    `Execute a shell command in the container for a service. Uses the web service by default. To run your command in the container for another service, run "lodev exec --service <service> <cmd>". If you want to use raw, uninterpreted command inside container use --raw as in example.`,
	Example: `lodev exec ls /var/www/html
lodev exec --service web
lodev exec -s web
lodev exec -s solr (assuming an add-on service named 'solr')
lodev exec -p my-project -s web (assuming a project exists named 'my-project')
lodev exec --raw -- ls -lR`,
	Run: func(cmd *cobra.Command, args []string) {
		activeApp, err := cmd.Flags().GetString("project")
		if err != nil {
			util.Failed("Failed to exec command: %v", err)
		}

		if len(args) == 0 {
			if err := cmd.Usage(); err != nil {
				util.Failed("Failed to display usage: %v", err)
			}
			os.Exit(1)
		}

		app, err := lodev.GetActiveProject(activeApp)
		if err != nil {
			util.Failed("Failed to exec command: %v", err)
		}

		if err = app.StartAppIfNotRunning(); err != nil {
			util.Failed("Failed to start project %s: %v", app.Name, err)
		}

		container, err := lodev.FindContainerByType(serviceType, app.GetName())
		if err != nil {
			util.Failed("Failed to find container for service '%s' in '%s' project: %v", serviceType, app.Name, err)
		}
		if container == nil {
			util.Failed("No running container found for service '%s' in '%s' project", serviceType, app.Name)
		}

		_ = app.DockerEnv()

		opts := &lodev.ExecOpts{
			Service: serviceType,
			Dir:     execDirArg,
			Cmd:     quoteArgs(args),
			Tty:     true,
			User:    serviceUser,
		}

		// If they've chosen raw, use the actual passed values.
		// Also, retrieve and preserve the current $PATH to ensure the environment is consistent.
		if cmd.Flag("raw").Changed {
			var env []string
			path, err := app.Exec(&lodev.ExecOpts{
				Service: serviceType,
				Cmd:     "echo $PATH",
				User:    serviceUser,
			})
			path = strings.Trim(path, "\n")
			if err == nil && path != "" {
				env = append(env, "PATH="+path)
			}
			// opts.RawCmd is used instead of opts.Cmd
			opts.RawCmd = args
			opts.Env = env
		}

		_, err = app.Exec(opts)
		quiet, _ := cmd.Flags().GetBool("quiet")

		if err != nil {
			exitCode := 1
			var statusErr cli.StatusError
			if errors.As(err, &statusErr) {
				exitCode = statusErr.StatusCode
			}
			if !quiet {
				util.Error("Failed to execute command `%s`: %v", opts.Cmd, err)
			}
			os.Exit(exitCode)
		}
	},
}

// quoteArgs quotes any arguments that contain spaces.
// This avoids splitting quoted strings with spaces into separate arguments.
// The function is adapted from the internal quoteArgs golang function.
func quoteArgs(args []string) string {
	if len(args) < 2 {
		return strings.Join(args, " ")
	}
	var b strings.Builder
	for i, arg := range args {
		if i > 0 {
			b.WriteString(" ")
		}
		if strings.ContainsAny(arg, "\" \t\r\n#") {
			b.WriteString(`"`)
			b.WriteString(strings.ReplaceAll(arg, `"`, `\"`))
			b.WriteString(`"`)
		} else {
			b.WriteString(arg)
		}
	}
	return b.String()
}

func init() {
	ExecCmd.Flags().StringVarP(&serviceType, "service", "s", "web", "Define the service to connect to. [e.g. web, db]")
	ExecCmd.Flags().StringVarP(&execDirArg, "dir", "d", "", "Define the execution directory within the container")
	ExecCmd.Flags().Bool("raw", true, "Use raw exec (do not interpret with Bash inside container)")
	ExecCmd.Flags().BoolP("quiet", "q", false, "Suppress detailed error output")
	ExecCmd.Flags().StringVarP(&serviceUser, "user", "u", "", "Defines the user to use within the container")
	ExecCmd.Flags().StringP("project", "p", "", "Project to use, defaults to the one for the current directory")
	// This requires flags for exec to be specified prior to any arguments, allowing for
	// flags to be ignored by cobra for commands that are to be executed in a container.
	ExecCmd.Flags().SetInterspersed(false)
	RootCmd.AddCommand(ExecCmd)
}
