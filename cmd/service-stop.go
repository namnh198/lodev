package cmd

import (
	"github.com/namnh198/lodev/internal/lodev"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var ServiceStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"down"},
	Short:   "Stop LODEV services",
	Example: `lodev service stop
lodev service stop --force
lodev service stop --update
	`,
	Args: cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		tap.Intro("🚀 Stopping LODEV services")
		err := lodev.StopLodevService()
		if err != nil {
			tap.Cancel("❌ Failed to stop LODEV services", tap.MessageOptions{
				Hint: err.Error(),
			})
			return
		}
		tap.Outro("✅ LODEV services are stopped!", tap.MessageOptions{
			Hint: "Use `lodev service` to check the status of your services",
		})
	},
}

func init() {
	ServiceCmd.AddCommand(ServiceStopCmd)
}
