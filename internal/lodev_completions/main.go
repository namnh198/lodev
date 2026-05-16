package main

import (
	"os"
	"path/filepath"

	"github.com/namnh198/lodev/cmd"
	"github.com/namnh198/lodev/pkg/util"
)

var targetDir = ".gotmp/completions"

func main() {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			util.Failed("Failed to create directory %s, ERR: %v", targetDir, err)
		}

		if err := cmd.RootCmd.GenBashCompletionFileV2(filepath.Join(targetDir, "lodev_bash_completions.bash"), true); err != nil {
			util.Failed("Could not generated lodev_bash_completion.bash. ERR: %v", err)
		}

		if err := cmd.RootCmd.GenZshCompletionFile(filepath.Join(targetDir, "lodev_zsh_completions.zsh")); err != nil {
			util.Failed("Could not generated lodev_zsh_completions.sh. ERR: %v", err)
		}

		if err := cmd.RootCmd.GenFishCompletionFile(filepath.Join(targetDir, "lodev_fish_completions.fish"), true); err != nil {
			util.Failed("could not generate lodev_fish_completions.fish: %v", err)
		}

		if err := cmd.RootCmd.GenPowerShellCompletionFileWithDesc(filepath.Join(targetDir, "lodev_powershell_completions.ps1")); err != nil {
			util.Failed("could not generate lodev_powershell_completions.ps1: %v", err)
		}
	}
}
