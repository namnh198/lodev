// Package cmd defines the lodev CLI commands.
package cmd

import (
	"github.com/spf13/cobra"

	"lodev/internal/ui"
)

var rootCmd = &cobra.Command{
	Use:           "lodev",
	Short:         "Lightweight local PHP dev environments on Docker with shared services",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if debug, _ := cmd.Flags().GetBool("debug"); debug {
			ui.SetDebug(true)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().Bool("debug", false, "print the docker commands lodev runs")
}

// StripDebugArg removes a leading --debug (right after the program name) and reports it,
// so it also works before docker-passthrough commands that disable flag parsing.
func StripDebugArg(args []string) ([]string, bool) {
	if len(args) > 1 && args[1] == "--debug" {
		return append([]string{args[0]}, args[2:]...), true
	}
	return args, false
}

// Execute runs the CLI.
func Execute() error {
	return rootCmd.Execute()
}
