package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/exec"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
)

const (
	CustomCommand        = "customCommand"
	BundledCustomCommand = "customCommand:bundled"
)

// addCustomCommands looks for custom command scripts in
// ~/.lodev/commands/<servicename> etc. and
// .lodev/commands/<servicename> and .lodev/commands/host
// and if it finds them adds them to Cobra's commands
func addCustomCommands(rootCmd *cobra.Command) error {
	// Keep a map so we don't add multiple commands with the same name.
	commandsAdded := map[string]int{}

	project, err := lodev.GetActiveProject("")
	if err != nil {
		return err
	}
	projectCommandPath := project.GetConfigPath("commands")
	lodevCommandPath := lodev.GetLodevConfigPath("commands")

	for _, commandSet := range []string{projectCommandPath, lodevCommandPath} {
		// If the item isn't a directory, skip it.
		if !fileutil.IsDirectory(commandSet) {
			continue
		}
		commandDirs, err := fileutil.ListFilesInDirFullPath(commandSet, false)
		if err != nil {
			return err
		}
		for _, dir := range commandDirs {
			// If the item isn't a directory, skip it.
			if !fileutil.IsDirectory(dir) {
				continue
			}
			// Skip hidden directories as well.
			if strings.HasPrefix(filepath.Base(dir), ".") {
				continue
			}
			commandFiles, err := fileutil.ListFilesInDir(dir)
			if err != nil {
				return err
			}
			err = addCustomCommandsFromDir(rootCmd, project, dir, commandFiles, commandsAdded)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// addCustomCommandsFromDir adds the custom commands from inside a given directory
func addCustomCommandsFromDir(rootCmd *cobra.Command, project *lodev.Project, dir string, commandFiles []string, commands map[string]int) error {
	service := filepath.Base(dir)
	var err error

	for _, commandName := range commandFiles {
		fullPath := filepath.Join(dir, commandName)
		if strings.HasSuffix(commandName, ".example") || strings.HasPrefix(commandName, "README") || strings.HasPrefix(commandName, ".") || fileutil.IsDirectory(fullPath) {
			continue
		}
		if _, ok := commands[commandName]; ok {
			continue
		}
		_ = util.Chmod(fullPath, 0755)
		directives := findDirectivesInCommandFile(fullPath)
		var description, usage, example, projectTypes, binary string

		// Skip host commands that need a project if we aren't in a project directory.
		if service == "host" && project == nil {
			if val, ok := directives["CanRunGlobally"]; !ok || val != "true" {
				if isCustomCommandInArgs(commandName) {
					util.Warning("Command '%s' cannot be used outside the project directory, skipping %s", commandName, fullPath)
				}
				continue
			}
		}

		usage = commandName + " [flags] [args]"
		if val, ok := directives["Usage"]; ok {
			usage = val
		}
		// Validate usage is not already in use
		if foundCmd, _, err := rootCmd.Find(strings.Split(usage, " ")); err == nil && foundCmd != nil {
			util.Warning("Command '%s' cannot have usage '%s' because it is already in use by command '%s', skipping %s", commandName, usage, foundCmd.Name(), fullPath)
			continue
		}

		description = commandName
		if val, ok := directives["Description"]; ok {
			description = val
		}
		if val, ok := directives["Example"]; ok {
			example = "  " + strings.ReplaceAll(val, `\n`, "\n  ")
		}
		var aliases []string
		if val, ok := directives["Aliases"]; ok {
			for alias := range strings.SplitSeq(val, ",") {
				alias = strings.TrimSpace(alias)
				if foundCmd, _, err := rootCmd.Find([]string{alias}); err != nil {
					aliases = append(aliases, alias)
				} else {
					util.Warning("Command '%s' cannot have alias '%s' that is already in use by command '%s', skipping alias for %s", commandName, alias, foundCmd.Name(), fullPath)
				}
			}
		}

		// Import and handle ProjectTypes
		if val, ok := directives["ProjectTypes"]; ok {
			projectTypes = val
		}

		// If ProjectTypes is specified and we aren't of that type, skip
		if projectTypes != "" && (project == nil || !strings.Contains(projectTypes, project.Type)) {
			if project != nil && isCustomCommandInArgs(commandName) {
				suggestedCommands := strings.Split(projectTypes, ",")
				for i, projectType := range suggestedCommands {
					suggestedCommands[i] = fmt.Sprintf("lodev config --project-type=%s", projectType)
				}
				suggestedCommand, _ := util.ArrayToReadableOutput(suggestedCommands)
				util.Warning("Command '%s' is not available for the '%s' project type, skipping %s.\nIf you intend to use '%s', change the project type to one of the supported types: %s", commandName, project.Type, fullPath, commandName, suggestedCommand)
			}
			continue
		}

		// Import and handle Binary
		if val, ok := directives["Binary"]; ok {
			binary = val
		}

		// If binary is specified it doesn't exist here, skip
		if binary != "" {
			binExists := false
			bins := strings.Split(binary, ",")
			if slices.ContainsFunc(bins, fileutil.FileExists) {
				binExists = true
			}
			if !binExists {
				if isCustomCommandInArgs(commandName) {
					suggestedBinaries, _ := util.ArrayToReadableOutput(bins)
					util.Warning("Command '%s' cannot be used, skipping %s\nThe binary is not found at: %s", commandName, fullPath, suggestedBinaries)
				}
				continue
			}
		}

		// Default is to exec with Bash interpretation (not raw)
		execRaw := false
		if val, ok := directives["ExecRaw"]; ok {
			if val == "true" {
				execRaw = true
			}
		}

		relative := false
		if val, ok := directives["HostWorkingDir"]; ok {
			if val == "true" {
				relative = true
			}
		}

		descSuffix := " (shell " + service + " container command)"

		if service == "host" {
			descSuffix = " (host command)"
		}

		// Initialize the new command
		command := &cobra.Command{
			Use:                usage,
			Short:              description + descSuffix,
			Example:            example,
			DisableFlagParsing: true,
			FParseErrWhitelist: cobra.FParseErrWhitelist{
				UnknownFlags: true,
			},
		}

		if service == "host" {
			command.Run = makeHostCmd(project, fullPath, commandName)
		} else {
			// Use path.Join() for the container path because it's about the path in the container, not on the
			// host; a Windows path is not useful here.
			containerBasePath := path.Join("/mnt/lodev_config", filepath.Base(filepath.Dir(dir)), service)
			inContainerFullPath := path.Join(containerBasePath, commandName)
			command.Run = makeContainerCmd(project, inContainerFullPath, commandName, service, execRaw, relative)
		}

		// Mark custom command
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}

		command.Annotations[CustomCommand] = "true"
		if lodev.IsBundledCustomCommand(service, commandName) {
			command.Annotations[BundledCustomCommand] = "true"
		}

		// Add the command and mark as added
		rootCmd.AddCommand(command)
		commands[commandName] = 1
	}

	return err
}

// isCustomCommandInArgs checks if the command is the first arg passed to the "lodev" command.
func isCustomCommandInArgs(commandName string) bool {
	return len(os.Args) > 1 && os.Args[1] == commandName
}

// findDirectivesInCommandFile returns a map of directives
// and their contents found in the named script
func findDirectivesInCommandFile(script string) map[string]string {
	f, err := os.Open(script)
	if err != nil {
		util.Failed("Failed to open %s: %v", script, err)
	}

	// nolint errcheck
	defer f.Close()
	var directives = make(map[string]string)
	// Splits on newlines by default.
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "##") && strings.Contains(line, ":") {
			line = strings.Replace(line, "## ", "", 1)
			parts := strings.SplitN(line, ":", 2)
			if parts[0] == "Example" {
				parts[1] = strings.Trim(parts[1], " ")
			} else {
				parts[1] = strings.Trim(parts[1], " \"'")
			}
			directives[parts[0]] = parts[1]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil
	}

	return directives
}

// makeHostCmd creates a command which will run on the host
func makeHostCmd(p *lodev.Project, fullPath, name string) func(*cobra.Command, []string) {
	var windowsBashPath = ""
	if util.IsWindows() {
		windowsBashPath = util.FindBashPath()
	}

	return func(_ *cobra.Command, _ []string) {
		if p != nil {
			status := p.SiteStatus()
			_ = p.DockerEnv()
			_ = os.Setenv("LODEV_PROJECT_STATUS", status)
		} else {
			_ = os.Setenv("LODEV_PROJECT_STATUS", "")
		}

		osArgs := []string{}
		if len(os.Args) > 2 {
			osArgs = os.Args[2:]
		}
		var err error
		// Load environment variables that may be useful for script.
		if p != nil {
			_ = p.DockerEnv()
		}

		if util.IsWindows() {
			// Sadly, not sure how to have a Bash interpreter without this.
			args := []string{fullPath}
			args = append(args, osArgs...)
			err = exec.RunInteractiveCommand(windowsBashPath, args)
		} else {
			err = exec.RunInteractiveCommand(fullPath, osArgs)
		}
		if err != nil {
			util.Failed("Failed to run %s %v; error=%v", name, strings.Join(osArgs, " "), err)
		}
	}
}

// makeContainerCmd creates the command which will app.Exec to a container command
func makeContainerCmd(project *lodev.Project, fullPath, name, service string, execRaw, relative bool) func(*cobra.Command, []string) {
	s := service
	if s[0:1] == "." {
		s = s[1:]
	}
	return func(_ *cobra.Command, _ []string) {
		status := project.SiteStatus()
		if status != lodev.SiteRunning {
			err := project.Start()
			if err != nil {
				util.Failed("Failed to start project for custom command: %v", err)
			}
		}
		_ = project.DockerEnv()

		osArgs := []string{}
		if len(os.Args) > 2 {
			osArgs = os.Args[2:]
		}

		opts := &lodev.ExecOpts{
			Cmd:       fullPath + " " + strings.Join(osArgs, " "),
			Service:   s,
			Dir:       project.GetWorkingDir(""),
			Tty:       true,
			NoCapture: true,
		}
		if relative {
			opts.Dir = path.Join(project.GetAbsAppRoot(true), project.GetRelativeWorkingDirectory())
		}

		if execRaw {
			opts.RawCmd = append([]string{fullPath}, osArgs...)
		}
		_, err := project.Exec(opts)

		if err != nil {
			util.Failed("Failed to run %s %v: %v", name, strings.Join(osArgs, " "), err)
		}
	}
}
