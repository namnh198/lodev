package dockerutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/docker/cli/cli-plugins/manager"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/cli/cli/version"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type DockerManager struct {
	goContext           context.Context            // Go context for Docker API calls
	apiClient           client.APIClient           // Docker API for making calls to the Docker daemon
	cli                 *command.DockerCli         // Docker CLI for getting dockerContextName and host
	dockerContextName   string                     // Current Docker context name (e.g: "default", "orbstack")
	host                string                     // Docker daemon host (e.g: "unix:///var/run/docker.sock")
	hostIP              string                     // IP address of Docker host (parsing from docker daemon host)
	hostErr             error                      // Error when looking up host IP address
	info                system.Info                // Docker system information from Docker daemon (Version, OS, etc)
	serverVersion       client.ServerVersionResult // Docker server information
	cliPlugins          []manager.Plugin           // Lazily discovered CLI plugins (e.g: "docker buildx", etc)
	cliPluginsExtraDirs []string                   // Extra directories to look for CLI plugins (e.g: ~/.docker/cli-plugins)
	cliPluginsErr       error                      // Erro when looking up CLI plugins
}

var (
	sDockerManager     *DockerManager // Singleton instance of DockerManager
	sDockerManagerErr  error          // Error when initializing the singleton DockerManager
	sDockerManagerOnce sync.Once      // Ensures that the DockerManger is only intialized once
)

// getDockerManagerInstance() returns a singleton instance of DockerManager, which is lazily initialized on the first call and cached for subsequent calls.
// It also returns any error that occurs during initialization.
func getDockerManagerInstance() (*DockerManager, error) {
	sDockerManagerOnce.Do(func() {
		sDockerManager = &DockerManager{}

		// Suppressing any output (stdout, stderr) from docker/cli
		sDockerManager.cli, sDockerManagerErr = command.NewDockerCli(
			command.WithCombinedStreams(io.Discard),
		)
		if sDockerManagerErr != nil {
			return
		}

		// InstallFlags and SetDefaultOptions are necessary to match
		// the plugin mode behavior to handle env vars such as
		// DOCKER_TLS and DOCKER_TLS_VERIFY.
		// See more: https://github.com/docker/cli/blob/master/cmd/docker-trust/trust/commands.go#L40
		nflags := pflag.NewFlagSet("lodev", pflag.ContinueOnError)
		options := flags.NewClientOptions()
		options.InstallFlags(nflags)
		options.SetDefaultOptions(nflags)
		sDockerManagerErr = sDockerManager.cli.Initialize(options)
		if sDockerManagerErr != nil {
			return
		}
		lodevDir := os.Getenv("LODEV_CONFIG_DIR")
		sDockerManager.cliPluginsExtraDirs = sDockerManager.cli.ConfigFile().CLIPluginsExtraDirs
		if lodevDir != "" {
			sDockerManager.cli.ConfigFile().CLIPluginsExtraDirs = append([]string{filepath.Join(lodevDir, "bin")}, sDockerManager.cliPluginsExtraDirs...)
		}

		sDockerManager.goContext = context.Background() // run with a background context
		sDockerManager.dockerContextName = sDockerManager.cli.CurrentContext()
		sDockerManager.host = sDockerManager.cli.DockerEndpoint().Host
		sDockerManager.hostIP, sDockerManagerErr = getDockerIPFromDockerHost(sDockerManager.host)
		// Set the Docker CLI version for UserAgent header
		version.Version = "lodev-" + nodeps.LodevVersion

		// We can't use SDockerMananger.cli.Client(), see: https://github.com/docker/cli/issues/4489
		// So we have to create a new API client using the same options as the Docker CLI
		sDockerManager.apiClient, sDockerManagerErr = command.NewAPIClientFromFlags(
			options, sDockerManager.cli.ConfigFile(),
		)
		if sDockerManagerErr != nil {
			return
		}

		sDockerManager.serverVersion, sDockerManagerErr = sDockerManager.apiClient.ServerVersion(sDockerManager.goContext, client.ServerVersionOptions{})
		if sDockerManagerErr != nil {
			return
		}

		info, sDockerManagerErr := sDockerManager.apiClient.Info(sDockerManager.goContext, client.InfoOptions{})

		if sDockerManagerErr != nil {
			return
		}

		sDockerManager.info = info.Info

		// A minimal cobra.Command is sufficient since we only need plugin
		// metadata, not builtin-command conflict detection.
		rootCmd := &cobra.Command{Use: "lodev"}
		sDockerManager.cliPlugins, sDockerManager.cliPluginsErr = manager.ListPlugins(sDockerManager.cli, rootCmd)
	})

	return sDockerManager, sDockerManagerErr
}

// GetDockerClient() returns Go Context and Docker API client for making calls to the Docker daemon
func GetDockerClient() (context.Context, client.APIClient, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return nil, nil, err
	}

	return dm.goContext, dm.apiClient, nil
}

// GetDockerClientInfo() returns Go Context and Docker API client
func GetDockerClientInfo() (system.Info, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return system.Info{}, err
	}

	return dm.info, nil
}

// GetDockerContextAndHost() returns the current Docker context name and Docker host URL
func GetDockerContextAndHost() (string, string, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return "", "", err
	}

	return dm.dockerContextName, dm.host, nil
}

// GetDockerIP() returns either the default Docker IP (127.0.0.1)
// or the value as configurated by Docker host (if it is a tcp:// URL)
func GetDockerIP() (string, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return "", err
	}

	return dm.hostIP, dm.hostErr
}

// GetServerVersion() return the Docker server cached version of Docker provided engine
// This is struct which call "docker version"
func GetServerVersion() (client.ServerVersionResult, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return client.ServerVersionResult{}, err
	}

	return dm.serverVersion, nil
}

// GetDockerVersion() return the Docker server cached version of Docker provided engine
func GetDockerVersion() (string, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return "", err
	}

	return dm.serverVersion.Version, nil
}

// GetDockerAPIVersion gets the cached API version of Docker provider engine
// See https://docs.docker.com/engine/api/#api-version-matrix
func GetDockerAPIVersion() (string, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return "", err
	}

	return dm.serverVersion.APIVersion, nil
}

// GetDockerCLIPlugins() returns the list of Docker CLI plugins installed on the system.
// Results are cached after the first call
func GetDockerCLIPlugins() ([]manager.Plugin, error) {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return nil, err
	}

	return dm.cliPlugins, dm.cliPluginsErr
}

// GetDockerCLIPlugin() returns the specified Docker CLI plugin by name, or an error if not found or if the plugin has an error.
func GetDockerCLIPlugin(name string) (*manager.Plugin, error) {
	plugins, err := GetDockerCLIPlugins()

	if err != nil {
		return &manager.Plugin{}, err
	}

	for _, plugin := range plugins {
		if plugin.Name == name {
			return &plugin, nil
		}
	}

	return &manager.Plugin{}, fmt.Errorf("docker CLI plugin %s not found", name)
}

// ResetDockerCLIPlugins resets the cached list of Docker CLI plugins in the singleton.
func ResetDockerCLIPlugins() error {
	dm, err := getDockerManagerInstance()
	if err != nil {
		return err
	}
	lodevDir := os.Getenv("LODEV_PATH")
	if lodevDir == "" {
		return nil
	}
	// Reset CLIPluginsExtraDirs, because lodevDir may have been modified by tests
	dm.cli.ConfigFile().CLIPluginsExtraDirs = append([]string{filepath.Join(lodevDir, "bin")}, dm.cliPluginsExtraDirs...)
	dm.cliPlugins, dm.cliPluginsErr = manager.ListPlugins(dm.cli, &cobra.Command{})
	return dm.cliPluginsErr
}

// IsRemoteDockerHost() check if the Docker host is remote by checking host IP address
// indicatiing that Docker daemon is running on a remote machine
func IsRemoteDockerHost() bool {
	dockerIP, err := GetDockerIP()
	if err != nil {
		return false
	}

	parseIP := net.ParseIP(dockerIP)

	if parseIP == nil || parseIP.IsLoopback() {
		return false
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.Contains(parseIP) {
			return false
		}
	}

	return true
}

// ResetDockerHost() resets cached Docker host data in singleton DockerManager instal (it's not safe)
// Used for testing only
func ResetDockerHost(host string) error {
	dm, err := getDockerManagerInstance()

	if err != nil {
		return err
	}

	dm.host = host
	dm.hostIP, dm.hostErr = getDockerIPFromDockerHost(host)

	return nil
}

// getDockerIPFromDockerHost() returns the IP address of the Docker host by parsing the Docker host URL
func getDockerIPFromDockerHost(host string) (string, error) {
	// default localhost
	hostIP := "127.0.0.1"
	dockerHostURL, err := url.Parse(host)
	if err != nil {
		return hostIP, err
	}

	hostPart := dockerHostURL.Hostname()

	if hostPart == "" {
		return hostIP, nil
	}

	// check to see if the hostname we found is an IP address
	addr := net.ParseIP(hostPart)

	// If it's not an IP addres, look it up to get the IP address
	if addr == nil {
		ip, err := net.DefaultResolver.LookupIP(context.Background(), "ipv4", hostPart)
		if err == nil && len(ip) > 0 {
			hostIP = ip[0].String()
		} else {
			return hostIP, fmt.Errorf("failed to look up IP address from host=%s, hostname=%s: %v", host, hostIP, err)
		}
	}

	return hostIP, nil
}

// CanRunWithoutDocker returns true if the command or flag can run without Docker.
func CanRunWithoutDocker() bool {
	if len(os.Args) < 2 {
		return true
	}
	// Some commands don't support Cobra help, because they are wrappers
	if slices.Contains([]string{"composer"}, os.Args[1]) {
		return false
	}
	// version and help should not require docker
	if util.ParseBoolFlag("version", "v") || util.ParseBoolFlag("help", "h") {
		return true
	}

	// help and hostname should not require docker
	if slices.Contains([]string{"help", "hostname"}, os.Args[1]) {
		return true
	}

	return false
}
