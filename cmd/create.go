package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var (
	projectNameArg       string
	projectTypeArg       string
	projectSkipPromptArg map[string]bool
)

var CreateCmd = &cobra.Command{
	Use:     "create [project-name]",
	Aliases: []string{"init", "add"},
	Short:   "Create a new LODEV project",
	Example: `lodev create
lodev create projectname
lodev create projectname --type=magento
	`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectSkipPromptArg = make(map[string]bool)
		pwd, err := os.Getwd()

		if err != nil {
			util.Failed("Could not detect the current directory: %v", err)
			return
		}

		if len(args) > 0 && args[0] != "" {
			projectNameArg = args[0]
		}
		if err := lodev.CanCreateProject(pwd); err != nil {
			util.Failed("Cannot create project in the current directory: %v", err)
		}

		lodev.WarnIfProjectExists(pwd)
		project, err := lodev.NewProject(pwd)
		if err != nil {
			util.Failed("Failed to create project: %v", err)
		}
		if err := handleBasicArgs(cmd, project); err != nil {
			util.Failed("Failed to create project: %v", err)
		}
		project.PromptCreateProject(projectSkipPromptArg)

		if err := project.ConfigFileOverrideAction(true); err != nil {
			util.Failed("Failed to override config: %v", err)
		}

		// Ensure the configuration passes validation before writing config file.
		if err := project.ValidateProjectConfig(); err != nil {
			util.Failed("Failed to validate config: %v", err)
		}

		if err := project.WriteProjectConfig(); err != nil {
			util.Failed("Could not write LODEV config file %s: %v", project.ConfigFile, err)
		}

		if cmd.Flag("start").Changed {
			util.SuccessMessage(fmt.Sprintf("Create a project %s sucessfully. Starting...", project.Name), tap.MessageOptions{
				Hint: fmt.Sprintf("You can manually edit the project configuration at %s", project.ConfigFile),
			})
			// hanle the case flag --start.
			return
		}

		tap.Outro(fmt.Sprintf("Create a project %s sucessfully. You may now run 'lodev start'.", project.Name), tap.MessageOptions{
			Hint: fmt.Sprintf("You can manually edit the project configuration at %s", project.ConfigFile),
		})
	},
}

func handleBasicArgs(cmd *cobra.Command, project *lodev.Project) (err error) {
	if project.Name != "" && projectNameArg == "" {
		// Sorry this is empty, but it makes the logic cleaner
	} else if projectNameArg != "" {
		projectSkipPromptArg["project-name"] = true
	} else {
		pwd, err := os.Getwd()
		if err != nil {
			return err
		}
		projectNameArg = filepath.Base(pwd)
	}
	project.Name = lodev.NormalizeProjectName(projectNameArg)

	if cmd.Flag("type").Changed && projectTypeArg != "" {
		if err = lodev.ValidateProjectType(projectTypeArg); err != nil {
			return
		}
		projectSkipPromptArg["project-type"] = true
		project.Type = projectTypeArg
	}

	return
}

func init() {
	CreateCmd.Flags().StringVar(&projectNameArg, "name", "", "Provide the project name of project to configure (default is current directory name)")
	CreateCmd.Flags().StringVar(&projectTypeArg, "type", "", "Provide the type of project to initialize, like 'magento2'")
	completionFunc(CreateCmd, "project-type", lodev.GetProjectTypes())
	CreateCmd.Flags().Bool("start", false, "Start the project right after creating it")
	completionFunc(CreateCmd, "start", []string{"true", "false"})
	RootCmd.AddCommand(CreateCmd)
}
