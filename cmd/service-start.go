package cmd

import (
	"github.com/namnh198/lodev/internal/lodev"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var ServiceStartCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"up"},
	Short:   "Start LODEV services",
	Example: `lodev service start
lodev service start --force
	`,
	Args: cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		var forceRefresh bool

		if cmd.Flag("force").Changed {
			forceRefresh = true
		}

		tap.Intro("🚀 Starting LODEV services")
		err := lodev.StartLodevService(true, forceRefresh)
		if err != nil {
			tap.Cancel("❌ Failed to start LODEV services", tap.MessageOptions{
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
	ServiceCmd.AddCommand(ServiceStartCmd)
	ServiceStartCmd.Flags().BoolP("force", "f", false, "Force stop and restart services even if they are already running")
}
