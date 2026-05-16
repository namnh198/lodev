package cmd

import (
	"fmt"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var RestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the project",
	Long:  "Restart the project by stopping and starting it again.",
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
		if err != nil {
			util.Failed("Failed to get project: %v", err)
		}
		tap.Intro(fmt.Sprintf("🔄 Restarting project %s...", projectName), tap.MessageOptions{
			Hint: fmt.Sprintf("In: %s", project.AppRoot),
		})

		tap.Message(fmt.Sprintf("Stopping project %s...", project.Name))
		if err := project.Stop(false); err != nil {
			tap.Cancel(fmt.Sprintf("Failed to stop project %s", project.Name), tap.MessageOptions{
				Hint: fmt.Sprintf("Error: %v", err),
			})
			return
		}
		if cmd.Flags().Changed("no-cache") {
			project.NoCache = true
		}
		tap.Message(fmt.Sprintf("Starting project %s...", project.Name))
		if err := project.Start(); err != nil {
			tap.Cancel(fmt.Sprintf("Failed to start project %s", project.Name), tap.MessageOptions{
				Hint: fmt.Sprintf("Error: %v", err),
			})
			return
		}

		tap.Outro(fmt.Sprintf("✅ Successfully restarted %s", project.Name), tap.MessageOptions{
			Hint: fmt.Sprintf("Your project can be reached at %s. See 'lodev show %s' for alternate URLs", project.GetPrimaryURL(), project.GetName()),
		})
	},
}

func init() {
	RestartCmd.Flags().StringP("name", "n", "", "Name of the project to restart. If not specified, it will restart the active project in the current directory")
	RestartCmd.Flags().StringSliceVarP(&composeProfilesArg, "profiles", "p", []string{}, "Start optional comma-separated docker compose profiles")
	RestartCmd.Flags().Bool("no-cache", false, "Start all projects with no-cache")

	RootCmd.AddCommand(RestartCmd)
}
