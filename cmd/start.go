package cmd

import (
	"fmt"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var composeProfilesArg []string

var StartCmd = &cobra.Command{
	Use:   "start [projectname]",
	Short: "Start the LODEV existing project",
	Long: `Start initializes and configures the web server and service containers
to provide a local development environment. You can run 'lodev start' from a
project directory to start that project, or you can start stopped projects in
any directory by running 'lodev start projectname [projectname ...]'`,
	Args: cobra.MaximumNArgs(1),
	Example: `lodev start
lodev start <project1> <project2>
lodev start --all`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		_, err := lodev.GetServiceList()
		if err != nil {
			return fmt.Errorf("Failed to get services registry: %v", err)
		}

		if err := lodev.EnsureLodevNetwork(); err != nil {
			return fmt.Errorf("Failed to ensure LODEV network: %v", err)
		}

		return err
	},
	Run: func(cmd *cobra.Command, args []string) {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		}

		project, err := lodev.GetActiveProject(projectName)
		if cmd.Flags().Changed("no-cache") {
			project.NoCache = true
		}
		if err != nil {
			util.Failed("Failed to start project: %v", err)
		}

		tap.Intro(fmt.Sprintf("🚀 Starting project %s...", project.Name), tap.MessageOptions{
			Hint: fmt.Sprintf("In: %s", project.AppRoot),
		})

		if err := project.Start(composeProfilesArg...); err != nil {
			tap.Outro(fmt.Sprintf("❌ Failed to start project %s", project.Name), tap.MessageOptions{
				Hint: fmt.Sprintf("Error: %v", err),
			})
			return
		}

		util.SuccessMessage(fmt.Sprintf("✅ Successfully started %s", project.Name), tap.MessageOptions{
			Hint: fmt.Sprintf("Your project can be reached at %s. See 'lodev show %s' for alternate URLs", project.GetPrimaryURL(), project.GetName()),
		})
	},
}

func init() {
	StartCmd.Flags().StringP("name", "n", "", "Name of the project to start. If not specified, it will start the active project in the current directory")
	StartCmd.Flags().StringSliceVarP(&composeProfilesArg, "profiles", "p", []string{}, "Start optional comma-separated docker compose profiles")
	StartCmd.Flags().Bool("no-cache", false, "Start project with no-cache")
	RootCmd.AddCommand(StartCmd)
}
