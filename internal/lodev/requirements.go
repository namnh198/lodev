package lodev

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/moby/moby/client/pkg/versions"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/network"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
)

// DockerVersionMatrix defines the minimum required versions for Docker
type DockerVersionMatrix struct {
	APIVersion               string
	Version                  string
	BuildxVersionConstraint  string
	ComposeVersionConstraint string
}

// DockerRequirements defines the minimum Docker version required by LODEV.
// We compare using the APIVersion because it's a consistent and reliable value.
// The Version is displayed to users as it's more readable and user-friendly.
// The values correspond to the API version matrix found here:
// https://docs.docker.com/reference/api/engine/#api-version-matrix
// BuildxVersion is in sync with https://docs.docker.com/buildx/working-with-buildx/#buildx-releases
// ComposeVersionConstraint is in sync with https://docs.docker.com/desktop/release-notes/
// The constraint MUST HAVE a -pre of some kind on it for successful comparison.
var DockerRequirements = DockerVersionMatrix{
	APIVersion:               "1.44",
	Version:                  "25.0",
	BuildxVersionConstraint:  ">= 0.17.0",
	ComposeVersionConstraint: ">= 2.24.3",
}

// CheckDokcerVersion determines if the Docker version of host system meets the minimum required version for LODEV
func CheckDockerVersion(requirement DockerVersionMatrix) error {
	dockerVer, err := dockerutil.GetDockerVersion()
	if err != nil {
		return err
	}

	dockerApiVer, err := dockerutil.GetDockerAPIVersion()
	if err != nil {
		return err
	}

	if !versions.GreaterThanOrEqualTo(dockerApiVer, requirement.APIVersion) {
		return fmt.Errorf("Installed docker %s is not supported, please update to the version %s or newer", dockerVer, requirement.Version)
	}

	return nil
}

// CheckDockerCompose determines if docker-compose is present and executable on the host system. This
// relies on docker-compose being somewhere in the user's $PATH.
func CheckDockerComposeVersion() error {
	_, err := DownloadDockerComposeIfNeeded()
	if err != nil {
		return err
	}
	versionConstraint := DockerRequirements.ComposeVersionConstraint

	v, err := GetDockerComposeVersion()
	if err != nil {
		return err
	}
	dockerComposeVersion, err := semver.NewVersion(v)
	if err != nil {
		return err
	}

	constraint, err := semver.NewConstraint(versionConstraint)
	if err != nil {
		return err
	}

	match, errs := constraint.Validate(dockerComposeVersion)
	if !match {
		if len(errs) <= 1 {
			return errs[0]
		}

		msgs := "\n"
		for _, err := range errs {
			msgs = fmt.Sprint(msgs, err, "\n")
		}
		return fmt.Errorf("%s", msgs)
	}

	return nil
}

// CheckDockerBuildx determines if docker-compose is present and executable on the host system. This
// relies on docker-compose being somewhere in the user's $PATH.
func CheckDockerBuildxVersion() error {
	_, err := DownloadDockerBuildxIfNeeded()
	if err != nil {
		return err
	}
	versionConstraint := DockerRequirements.BuildxVersionConstraint

	v, err := GetDockerBuildxVersion()
	if err != nil {
		return err
	}
	dockerComposeVersion, err := semver.NewVersion(v)
	if err != nil {
		return err
	}

	constraint, err := semver.NewConstraint(versionConstraint)
	if err != nil {
		return err
	}

	match, errs := constraint.Validate(dockerComposeVersion)
	if !match {
		if len(errs) <= 1 {
			return errs[0]
		}

		msgs := "\n"
		for _, err := range errs {
			msgs = fmt.Sprint(msgs, err, "\n")
		}
		return fmt.Errorf("%s", msgs)
	}

	return nil
}

// DownloadDockerComposeIfNeeded downloads the proper version of docker-compose
// if it's either not yet installed or has the wrong version.
// Returns downloaded bool (true if it did the download) and err
func DownloadDockerComposeIfNeeded() (bool, error) {
	requiredVersion := nodeps.RequiredDockerComposeVerion
	var err error
	if requiredVersion == "" {
		return false, nil
	}
	curVersion, err := GetDockerComposeVersion()
	if err != nil || curVersion != requiredVersion {
		err = DownloadDockerCompose()
		if err == nil {
			return true, err
		}
	}
	return false, err
}

// GetDockerComposeVersion runs docker-compose -v to get the current version and cached result
func GetDockerComposeVersion() (string, error) {
	if nodeps.DockerComposeVersion != "" {
		return nodeps.DockerComposeVersion, nil
	}

	composePath, err := GetDockerComposePath()
	if err != nil {
		return "", err
	}

	if !fileutil.FileExists(composePath) {
		nodeps.DockerComposeVersion = ""
		return "", fmt.Errorf("docker-compose does not exist at %s", composePath)
	}

	out, err := exec.Command(composePath, "version", "--short").Output()
	if err != nil {
		return "", err
	}

	composerVer := strings.Trim(string(out), "\r\n")

	if !strings.HasPrefix(composerVer, "v") {
		composerVer = "v" + composerVer
	}

	nodeps.DockerComposeVersion = composerVer

	return composerVer, nil
}

// GetDockerComposePath gets the full path to the docker-compose binary
// Normally this is the one that has been downloaded to ~/.lodev/bin, but if
// UseDockerComposeFromPath, then it will be whatever if found in $PATH
func GetDockerComposePath() (string, error) {
	executableName := "docker-compose"
	if LodevConfig.UseDockerComposeSystem {
		path, err := exec.LookPath(executableName)
		if err != nil {
			return "", fmt.Errorf("docker-compose not found in PATH: %w", err)
		}

		return path, nil
	}

	if util.IsWindows() {
		executableName += ".exe"
	}

	return GetLodevBinPath(executableName), nil
}

// DownloadDockerCompose gets the docker-compose binary and puts it into
// ~/.lodev/.bin
func DownloadDockerCompose() error {
	lodevBinDir := GetLodevBinPath("")
	destFile, _ := GetDockerComposePath()

	composeURL, shasumURL, err := dockerComposeDownloadLink()
	if err != nil {
		return err
	}

	_ = os.Remove(destFile)

	_ = os.MkdirAll(lodevBinDir, 0777)
	err = network.DownloadFile(destFile, composeURL, true, shasumURL)
	if err != nil {
		_ = os.Remove(destFile)
		return err
	}

	// Remove the cached DockerComposeVersion
	nodeps.DockerComposeVersion = ""

	err = util.Chmod(destFile, 0755)
	if err != nil {
		return err
	}

	return nil
}

// dockerComposeDownloadLink returns the URL and SHASUM-file link for docker-compose
func dockerComposeDownloadLink() (composeURL string, shasumURL string, err error) {
	arch := runtime.GOARCH

	switch arch {
	case "arm64":
		arch = "aarch64"
	case "amd64":
		arch = "x86_64"
	default:
		return "", "", fmt.Errorf("only ARM64 and AMD64 architectures are supported for docker-compose, not %s", arch)
	}
	flavor := runtime.GOOS + "-" + arch
	composerURL := fmt.Sprintf("https://github.com/docker/compose/releases/download/%s/docker-compose-%s", nodeps.RequiredDockerComposeVerion, flavor)
	if util.IsWindows() {
		composerURL = composerURL + ".exe"
	}
	shasumURL = fmt.Sprintf("https://github.com/docker/compose/releases/download/%s/checksums.txt", nodeps.RequiredDockerComposeVerion)

	return composerURL, shasumURL, nil
}

// DownloadDockerBuildxIfNeeded downloads the proper version of docker-buildx
// if it's either not yet installed or has the wrong version.
// Returns downloaded bool (true if it did the download) and err
func DownloadDockerBuildxIfNeeded() (bool, error) {
	requiredVersion := nodeps.RequiredDockerBuildxVersion
	var err error
	if requiredVersion == "" {
		return false, nil
	}
	curVersion, err := GetDockerBuildxVersion()
	if err != nil || curVersion != requiredVersion {
		err = DownloadDockerBuildx()
		if err == nil {
			return true, err
		}
	}
	return false, err
}

// GetDockerBuildxVersion runs docker-compose -v to get the current version and cached result
func GetDockerBuildxVersion() (string, error) {
	plugin, err := dockerutil.GetDockerCLIPlugin("buildx")
	if err != nil {
		return "", err
	}

	return plugin.Version, nil
}

// GetDockerComposePath gets the full path to the docker-compose binary
// Normally this is the one that has been downloaded to ~/.lodev/bin, but if
// UseDockerComposeFromPath, then it will be whatever if found in $PATH
func GetDockerBuildxPath() (string, error) {
	plugin, err := dockerutil.GetDockerCLIPlugin("buildx")
	if err == nil && plugin.Path != "" {
		return plugin.Path, nil
	}

	pluginPath := GetLodevBinPath("docker-buildx")

	return pluginPath, nil
}

// DownloadDockerCompose gets the docker-compose binary and puts it into
// ~/.lodev/.bin
func DownloadDockerBuildx() error {
	destFile := GetLodevBinPath("docker-buildx")

	composeURL, shasumURL, err := dockerBuildxDownloadLink()
	if err != nil {
		return err
	}

	_ = os.Remove(destFile)

	_ = os.MkdirAll(filepath.Dir(destFile), 0777)
	err = network.DownloadFile(destFile, composeURL, true, shasumURL)
	if err != nil {
		_ = os.Remove(destFile)
		return err
	}

	// Remove the cached DockerComposeVersion
	nodeps.DockerComposeVersion = ""

	err = util.Chmod(destFile, 0755)
	if err != nil {
		return err
	}

	if err := dockerutil.ResetDockerCLIPlugins(); err != nil {
		return err
	}

	if _, err := GetDockerBuildxVersion(); err != nil {
		return err
	}

	return nil
}

// dockerComposeDownloadLink returns the URL and SHASUM-file link for docker-compose
func dockerBuildxDownloadLink() (composeURL string, shasumURL string, err error) {
	arch := runtime.GOARCH
	buildxVer := nodeps.RequiredDockerBuildxVersion
	flavor := runtime.GOOS + "-" + arch
	composerURL := fmt.Sprintf("https://github.com/docker/buildx/releases/download/%s/buildx-%s.%s", buildxVer, buildxVer, flavor)
	if util.IsWindows() {
		composerURL = composerURL + ".exe"
	}
	shasumURL = fmt.Sprintf("https://github.com/docker/buildx/releases/download/%s/checksums.txt", buildxVer)

	return composerURL, shasumURL, nil
}
