package cmd

import (
	"fmt"
	"strings"

	"github.com/namnh198/lodev/internal/lodev"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
	"github.com/yarlson/tap"
)

var ShowCmd = &cobra.Command{
	Use:     "show",
	Aliases: []string{"describe", "info", "status"},
	Short:   "Show LODEV information",
	Long:    "Show LODEV information such as current configuration, project status, etc.",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		}

		project, err := lodev.GetActiveProject(projectName)
		if err != nil {
			util.Failed("Failed to get project: %v.\nYou may try to run 'lodev create'", err)
		}
		// Ensure we have all services to describe for never-started projects.
		if !fileutil.FileExists(project.DockerComposeFullRenderedYAMLPath()) {
			_ = project.DockerEnv()
			err = project.WriteDockerComposeYAML()
			if err != nil {
				util.Failed("Failed to run `docker-compose config` for '%s': %v", project.Name, err)
			}
		}

		tap.Box(
			func() string {
				var buf strings.Builder
				highlight := func(ansi, text string) string {
					return fmt.Sprintf("%s%s%s", ansi, text, tap.Reset)
				}
				dmContext, _, _ := dockerutil.GetDockerContextAndHost()
				nameDesc := fmt.Sprintf("(%s)", project.GetShortAppRoot())
				typeDesc := fmt.Sprintf("(PHP:%s %s)", project.PHPVersion, project.Webserver)
				_, _ = fmt.Fprintf(&buf, "%sProject: %s%s %s\n", tap.Cyan, tap.Reset, highlight(tap.Green, project.Name), highlight(tap.Dim, nameDesc))
				_, _ = fmt.Fprintf(&buf, "%sProject Type: %s%s %s\n", tap.Cyan, tap.Reset, highlight(tap.Green, project.Type), highlight(tap.Dim, typeDesc))
				_, _ = fmt.Fprintf(&buf, "%sDocker Platform: %s%s\n", tap.Cyan, tap.Reset, highlight(tap.Red, dmContext))
				_, _ = fmt.Fprintf(&buf, "%sRouter:%s %s\n", tap.Cyan, tap.Reset, highlight(tap.Green, lodev.LodevConfig.Router))
				_, _ = fmt.Fprintf(&buf, "%sLODEV Version: %s%s", tap.Cyan, highlight(tap.Yellow, nodeps.LodevVersion), tap.Reset)
				return buf.String()
			}(),
			"  PROJECT SUMMARY  ",
			tap.BoxOptions{
				Columns:        90,
				WidthFraction:  1.0,
				TitlePadding:   1,
				ContentPadding: 1,
				Rounded:        true,
				FormatBorder:   tap.CyanBorder,
			},
		)
	},
}

func init() {
	RootCmd.AddCommand(ShowCmd)
}
