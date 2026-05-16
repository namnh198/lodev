package cmd

import (
	"github.com/namnh198/lodev/internal/lodev"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var PoweroffCmd = &cobra.Command{
	Use:     "poweroff",
	Aliases: []string{"powerdown"},
	Short:   "Completely stop all projects and containers",
	Long:    "Power off the current LODEV environment, stopping all running router, projects and services.",
	Run: func(cmd *cobra.Command, args []string) {
		tap.Intro("Poweroff LODEV services")
		lodev.PowerOff()
		tap.Outro("Done!!!")
	},
}

func init() {
	RootCmd.AddCommand(PoweroffCmd)
}
