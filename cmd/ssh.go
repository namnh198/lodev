package cmd

import (
	"os"
	"os/exec"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
)

var (
	sshDirArg  string
	sshUserArg string
)

var SSHCommand = &cobra.Command{
	Use:   "ssh",
	Short: "Starts a shell session in the container for a service. Uses web service by default.",
	Example: `lodev ssh
lodev ssh <projectname>
lodev ssh -s web
lodev ssh -d /var/www/html
`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		}

		project, err := lodev.GetActiveProject(projectName)
		if err != nil {
			util.Failed("Failed to get project: %v", err)
		}

		if err = project.StartAppIfNotRunning(); err != nil {
			util.Failed("Failed to start project %s: %v", project.Name, err)
		}

		err = project.Attach(&lodev.ExecOpts{
			Service: nodeps.WebContainer,
			Cmd:     "bash -l",
			Dir:     sshDirArg,
			User:    sshUserArg,
		})

		if err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				os.Exit(exiterr.ExitCode())
			}
			util.Failed("ssh ssh failed: %v", err)
		}
	},
}

func init() {
	SSHCommand.Flags().StringVarP(&sshDirArg, "dir", "d", "", "The directory to start the shell session in.")
	SSHCommand.Flags().StringVarP(&sshUserArg, "user", "u", "", "The user to run as inside the container.")
	RootCmd.AddCommand(SSHCommand)
}
