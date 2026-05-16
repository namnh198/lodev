package cmd

import (
	"os"
	"slices"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
)

var (
	serviceType string
	serviceUser string
)

var RootCmd = &cobra.Command{
	Use:     "lodev",
	Short:   "LODEV is local web development environment.",
	Long:    "Create and manage local web development enviroment easily.",
	Version: nodeps.LodevVersion,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if cmd.Flag("verbose").Changed {
			os.Setenv("LODEV_VERBOSE", "true")
		}

		// Ensure LodevConfig is loaded before executing any command
		lodev.EnsureLodevConfig()

		if len(os.Args) < 2 {
			return
		}
		command := os.Args[1]

		if dockerutil.CanRunWithoutDocker() {
			return
		}

		if !slices.Contains([]string{"start", "stop", "service"}, command) {
			return
		}

		if err := lodev.CheckDockerBuildxVersion(); err != nil {
			util.Failed("Docker buildx check failed: %v", err)
		}

		if err := lodev.CheckDockerComposeVersion(); err != nil {
			util.Failed("Docker compose check failed: %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// fallback to help message
		cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(0)
	}
}

func init() {
	RootCmd.PersistentFlags().BoolP("verbose", "V", false, "Enable verbose output")

	if !dockerutil.CanRunWithoutDocker() {
		_, err := dockerutil.GetDockerVersion()
		if err != nil {
			util.Failed("Docker Error. Please check again. ERR: %v", err)
		}
	}

	// Set $DOCKER_CLI_HINTS environment variable to false to disable docker CLI hints
	_ = os.Setenv("DOCKER_CLI_HINTS", "false")
	// Populate the assets and commands so they're visible
	// Skip populating when running with root privileges since the assets and commands are only needed for non-root users
	if os.Geteuid() != 0 {
		err := lodev.PopulateLodevAssetsAndCommands("")
		if err != nil {
			util.Warning("PopulateLodevAssetsAndCommand() failed: %v", err)
		}
		addCustomCommands(RootCmd)
	}
}
