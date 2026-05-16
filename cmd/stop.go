package cmd

import (
	"fmt"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var StopCmd = &cobra.Command{
	Use:     "stop [project-name]",
	Aliases: []string{"remove", "delete"},
	Short:   "Stop an existing LODEV project",
	Long:    "Stop an existing LODEV project with the specified name.",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		}

		project, err := lodev.GetActiveProject(projectName)
		if err != nil {
			util.Failed("Failed to start project: %v", err)
		}

		removeData := cmd.Flags().Changed("delete")

		tap.Intro(fmt.Sprintf("🛑 Stopping project %s...", project.Name))

		if err := project.Stop(removeData); err != nil {
			tap.Cancel(fmt.Sprintf("❌ Failed to stop project %s", project.Name), tap.MessageOptions{
				Hint: fmt.Sprintf("Error: %v", err),
			})
			return
		}

		tap.Outro(fmt.Sprintf("✅ Project %s stopped successfully.", project.Name), tap.MessageOptions{})
	},
}

func init() {
	StopCmd.Flags().BoolP("delete", "d", false, "Delete project after stopping")
	RootCmd.AddCommand(StopCmd)
}
