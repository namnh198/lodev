// Package cmd defines the lodev CLI commands.
package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:           "lodev",
	Short:         "Lightweight local PHP dev environments on Docker with shared services",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI.
func Execute() error {
	return rootCmd.Execute()
}
