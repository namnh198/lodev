package cmd

import (
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/dockertools"
)

// Version is set at build time: go build -ldflags "-X lodev/cmd.Version=v1.2.0".
var Version = "dev"

// Channel is "edge" for the pre-release built from main (-X lodev/cmd.Channel=edge), else "".
var Channel = ""

// lodevVersion is Version, or "dev (<commit>[-dirty])" from the build info Go embeds.
func lodevVersion() string {
	if Version != "dev" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	var rev string
	dirty := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return Version
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "-dirty"
	}
	return Version + " (" + rev + ")"
}

// dockerValue runs a docker query; a failure (e.g. Docker not running) reads "unavailable".
func dockerValue(args ...string) string {
	out, err := dockerOutput(args...)
	if err != nil || out == "" {
		return "unavailable"
	}
	return out
}

// toolVersion is the compose or buildx version lodev runs, without downloading anything.
func toolVersion(t dockertools.Tool, system bool, query ...string) string {
	if !system {
		return t.Version() + " (lodev)"
	}
	v := dockerValue(query...)
	if f := strings.Fields(v); t == dockertools.Buildx && len(f) > 1 {
		v = f[1] // "github.com/docker/buildx v0.37.2 <commit>"
	}
	if v == "unavailable" {
		return v
	}
	return "v" + strings.TrimPrefix(v, "v") + " (system)"
}

func versionRows() [][]string {
	g, err := config.LoadGlobal()
	if err != nil {
		g = &config.Global{} // the managed default, as docker.command does
	}
	return [][]string{
		{"lodev", lodevVersion()},
		{"Docker", dockerValue("version", "--format", "{{.Server.Version}}")},
		{"Docker Compose", toolVersion(dockertools.Compose, g.UseDockerComposeSystem, "compose", "version", "--short")},
		{"Docker Buildx", toolVersion(dockertools.Buildx, g.UseDockerBuildxSystem, "buildx", "version")},
		{"OS", runtime.GOOS},
		{"Architecture", runtime.GOARCH},
	}
}

func init() {
	rootCmd.Version = lodevVersion() // cobra adds -v/--version: "lodev version <v>"
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show lodev, Docker and platform versions",
		Args:  cobra.NoArgs,
		Run: func(c *cobra.Command, _ []string) {
			printTable(c.OutOrStdout(), []string{"COMPONENT", "VERSION"}, versionRows())
		},
	})
}
