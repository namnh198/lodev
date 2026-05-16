package cmd

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
)

// ComposerCmd handles lodev composer
var ComposerCmd = &cobra.Command{
	DisableFlagParsing: true,
	Use:                "composer [command]",
	Short:              "Executes a Composer command within the web container",
	Long: `Executes a Composer command at the Composer root in the web container. Generally,
any Composer command can be forwarded to the container context by prepending
the command with 'lodev'.`,
	Aliases: []string{"co"},
	Example: `lodev composer install
lodev composer require <package>
lodev composer outdated --minor-only
lodev composer create-project drupal/recommended-project .`,
	Run: func(_ *cobra.Command, args []string) {
		project, err := lodev.GetActiveProject("")
		if err != nil {
			util.Failed("Failed to get active project: %v", err)
		}

		status := project.SiteStatus()
		if status != lodev.SiteRunning {
			if err = project.Start(); err != nil {
				util.Failed("Failed to start %s: %v", project.Name, err)
			}
		}

		showComposerWarningForNotPersistentChanges(args)

		stdout, err := project.Composer(args)
		if err != nil {
			util.Failed("Composer %v failed, %v", args, err)
		}
		_, _ = fmt.Fprint(os.Stdout, stdout)
	},
}

func init() {
	ComposerCmd.InitDefaultHelpFlag()
	ComposerCmd.Flags().MarkHidden("help")
	RootCmd.AddCommand(ComposerCmd)
}

func showComposerWarningForNotPersistentChanges(args []string) {
	// Find the first actual command argument (skip flags, handle -- separator)
	command := ""
	for i, arg := range args {
		if arg == "--" {
			if i+1 < len(args) {
				command = args[i+1]
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		command = arg
		break
	}

	// Warn for commands that modify the container in non-persistent ways
	if !slices.Contains([]string{"self-update", "selfupdate", "global"}, command) {
		return
	}

	util.Warning("Composer %s changes do not persist across lodev restarts.\nSee https://docs.lodev.com/en/stable/users/usage/developer-tools/#composer-limitations for details.", command)
}
