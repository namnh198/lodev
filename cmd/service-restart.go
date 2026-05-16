package cmd

import (
	"github.com/namnh198/lodev/internal/lodev"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var ServiceRestartCmd = &cobra.Command{
	Use:     "restart",
	Short:   "Restart LODEV services",
	Example: "lodev service restart",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		tap.Intro("🚀 Starting LODEV services")
		err := lodev.StartLodevService(true, true)
		if err != nil {
			tap.Cancel("❌ Failed to restart LODEV services", tap.MessageOptions{
				Hint: err.Error(),
			})
			return
		}
		tap.Outro("✅ LODEV services are up and running!", tap.MessageOptions{
			Hint: "Use `lodev service` to check the status of your services",
		})
	},
}

func init() {
	ServiceCmd.AddCommand(ServiceRestartCmd)
}
