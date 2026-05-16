package cmd

import (
	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
)

var LogsCmd = &cobra.Command{
	Use:     "logs",
	Aliases: []string{"log"},
	Short:   "View the logs of the running project",
	Long:    `Logs shows the combined logs of all running containers in the project.`,
	Args:    cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		}

		project, err := lodev.GetActiveProject(projectName)
		if err != nil {
			util.Failed("Failed to get active project: %v", err)
		}

		if err := project.Logs("web", false, false, ""); err != nil {
			util.Failed("Failed to get logs for project %s: %v", project.Name, err)
		}
	},
}

func init() {
	RootCmd.AddCommand(LogsCmd)
}
