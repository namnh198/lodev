package lodev

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/joho/godotenv"
	"github.com/moby/moby/client"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/hostname"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/yarlson/tap"
)

// List app statuses
const (
	SiteStarting      = "starting"
	SiteStopped       = "stopped"
	SiteRunning       = "running"
	SitePaused        = "paused"
	SiteDirMissing    = "project directory is missing"
	SiteHealthy       = "healthy"
	SiteUnhealthy     = "unhealthy"
	SiteConfigMissing = ".lodev/config.yml is missing"
)

// ProjectWebExtraExposedPorts is the struct that represents an extra port that will be exposed from the web container (e.g. exposed port 3000 for npm run dev, etc.)
type ProjectWebExtraExposedPorts struct {
	Name          string `yaml:"name"`
	ContainerPort int    `yaml:"container_port"`
	HTTPPort      int    `yaml:"http_port"`
	HTTPSPort     int    `yaml:"https_port"`
}

// ProjectWebExtraDaemon is the struct that represents a daemon that will be run in the web container (e.g. npm run dev, php artisan serve, etc.)
type ProjectWebExtraDaemon struct {
	Name      string `yaml:"name"`
	Command   string `yaml:"command"`
	Directory string `yaml:"directory"`
}

type Crontab struct {
	Schedule string `yaml:"schedule"`
	Command  string `yaml:"command"`
}

// Project is the struct that represents a LODEV project, mostly its config in .lodev/config.yaml
type Project struct {
	Name                 string                        `yaml:"name"`
	Type                 string                        `yaml:"type"`
	Docroot              string                        `yaml:"docroot"`
	PHPVersion           string                        `yaml:"php_version"`
	Webserver            string                        `yaml:"webserver"`
	XdebugEnabled        bool                          `yaml:"xdebug_enabled"`
	NodeJSVersion        string                        `yaml:"nodejs_version"`
	HostWebserverPort    string                        `yaml:"host_webserver_port,omitempty"`
	HostHttpsPort        string                        `yaml:"host_https_port,omitempty"`
	ComposerVersion      string                        `yaml:"composer_version"`
	RestartAlways        bool                          `yaml:"restart_always"`
	AdditionalHosts      []string                      `yaml:"additional_hosts"`
	WorkingDir           string                        `yaml:"working_dir"`
	ComposerRoot         string                        `yaml:"composer_root,omitempty"`
	Crontab              []Crontab                     `yaml:"crontab"`
	WebEnvironment       []string                      `yaml:"web_enviroment"`
	ConnectedServices    []string                      `yaml:"connected_services"`
	Timezone             string                        `yaml:"timezone,omitempty"`
	WebExtraExposedPorts []ProjectWebExtraExposedPorts `yaml:"web_extra_exposed_ports,omitempty"`
	WebExtraDaemons      []ProjectWebExtraDaemon       `yaml:"web_extra_daemons,omitempty"`
	AppRoot              string                        `yaml:"-"`
	ConfigFile           string                        `yaml:"-"`
	WebImage             string                        `yaml:"-"`
	ComposeYAML          *composeTypes.Project         `yaml:"-"`
	NoCache              bool                          `yaml:"-"`
	Status               string                        `yaml:"-"`
}

// GetName returns the name of the project
func (p *Project) GetName() string {
	if p.Name == "" {
		currentDir, _ := os.Getwd()
		p.Name = NormalizeProjectName(filepath.Base(currentDir))
	}

	return p.Name
}

// GetType returns the type of the project
func (p *Project) GetType() string {
	return p.Type
}

// GetDocroot returns the docroot of the project, if not set, it will try to detect it and set it in the project struct
func (p *Project) GetDocroot() string {
	if p.Docroot != "" {
		return p.Docroot
	}
	p.DetectDocroot()
	return p.Docroot
}

func (p *Project) GetShortAppRoot() string {
	return fileutil.ShortHomeJoin(p.AppRoot)
}

// GetHostname returns the primary hostname of the app.
func (p *Project) GetHostname() string {
	return strings.ToLower(p.Name) + "." + LodevConfig.ProjectTld
}

// GetHostnames returns a slice of all the configured hostnames.
func (p *Project) GetHostnames() []string {
	// Use a map to make sure that we have unique hostnames
	// The value is useless, so use the int 1 for assignment.
	nameListMap := make(map[string]int)
	nameListArray := []string{}

	if !IsRouterDisabled() {
		for _, name := range p.AdditionalHosts {
			name = strings.ToLower(name)
			nameListMap[name+"."+LodevConfig.ProjectTld] = 1
		}

		// Make sure the primary hostname didn't accidentally get added, it will be prepended
		delete(nameListMap, p.GetHostname())

		// Now walk the map and extract the keys into an array.
		for k := range nameListMap {
			nameListArray = append(nameListArray, k)
		}
		sort.Strings(nameListArray)
		// We want the primary hostname to be first in the list.
		nameListArray = append([]string{p.GetHostname()}, nameListArray...)
	}
	return nameListArray
}

// GetAbsDocroot returns the absolute path to the docroot on the host or if
// inContainer is set to true in the container.
func (p *Project) GetAbsDocroot(inContainer bool) string {
	if inContainer {
		return path.Join(p.GetAbsAppRoot(inContainer), p.GetDocroot())
	}

	return filepath.Join(p.GetAbsAppRoot(inContainer), p.GetDocroot())
}

// GetAbsAppRoot returns the absolute path to the project root on the host or if
// inContainer is set to true in the container.
func (p *Project) GetAbsAppRoot(inContainer bool) string {
	if inContainer {
		return p.WorkingDir
	}

	return p.AppRoot
}

// GetComposerRoot will determine the absolute Composer root directory where
// all Composer related commands will be executed.
// If inContainer set to true, the absolute path in the container will be
// returned, else the absolute path on the host.
// If showWarning set to true, a warning containing the Composer root will be
// shown to the user to avoid confusion.
func (p *Project) GetComposerRoot(inContainer, showWarning bool) string {
	var absComposerRoot string

	if inContainer {
		absComposerRoot = path.Join(p.GetAbsAppRoot(true), p.ComposerRoot)
	} else {
		absComposerRoot = filepath.Join(p.GetAbsAppRoot(false), p.ComposerRoot)
	}

	// If requested, let the user know we are not using the default Composer
	// root directory to avoid confusion.
	if p.ComposerRoot != "" && showWarning {
		util.Warning("Using '%s' as Composer root directory", absComposerRoot)
	}

	return absComposerRoot
}

// GetComposeProjectName returns the name of the docker-compose project
func (p *Project) GetComposeProjectName() string {
	return strings.ToLower("lodev-" + strings.ReplaceAll(p.Name, `.`, ""))
}

// GetPrimaryURL returns the primary URL that can be used, https or http
// inContainer is set to true in the container.
func (p *Project) GetPrimaryURL() string {
	scheme := "http"
	if CanUseHTTPS() {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, p.GetHostname())
}

func (p *Project) GetNetworkName() string {
	return p.GetComposeProjectName() + "_default"
}

// GetPublishedPort returns the host-exposed public port of a container.
func (p *Project) GetPublishedPort(serviceName string) (int, error) {
	exposedPort := "80"
	exposedPortInt, err := strconv.Atoi(exposedPort)
	if err != nil {
		return -1, err
	}
	return p.GetPublishedPortForPrivatePort(serviceName, uint16(exposedPortInt))
}

// GetPublishedPortForPrivatePort returns the host-exposed public port of a container for a given private port.
func (p *Project) GetPublishedPortForPrivatePort(serviceName string, privatePort uint16) (publicPort int, err error) {
	c, err := FindContainerByType(serviceName, p.GetName())
	if err != nil || c == nil {
		return -1, fmt.Errorf("failed to find container of type web: %v", err)
	}
	publishedPort := dockerutil.GetPublishedPort(privatePort, *c)
	return publishedPort, nil
}

// ComposeFiles returns a list of compose files for a project.
// It has to put the .lodev/docker-compose.*.y*ml first
// It has to put the .lodev/docker-compose.override.y*ml last
func (p *Project) ComposeFiles() ([]string, error) {
	userFiles, err := filepath.Glob(p.GetConfigPath("docker-compose.*.y*ml"))
	if err != nil {
		return []string{}, err
	}
	files := append([]string{p.DockerComposeYAMLPath()}, userFiles...)
	overrideFiles, err := filepath.Glob(p.GetConfigPath("docker-compose.override.y*ml"))
	if err != nil {
		return []string{}, err
	}
	files = append(files, overrideFiles...)
	return files, nil
}

// GetHostWorkingDir will determine the appropriate working directory for the service on the host side
func (p *Project) GetHostWorkingDir(dir string) string {
	if dir == "" && p.WorkingDir != "" {
		dir = p.WorkingDir
	}
	containerWorkingDirPrefix := strings.TrimSuffix(p.GetAbsAppRoot(true), "/") + "/"
	if !strings.HasPrefix(dir, containerWorkingDirPrefix) {
		return ""
	}
	return filepath.Join(p.GetAbsAppRoot(false), strings.TrimPrefix(dir, containerWorkingDirPrefix))
}

// GetWorkingDir will determine the appropriate working directory for an Exec/ExecWithTty command
// by consulting with the project configuration. If no dir is specified for the service, an
// empty string will be returned.
func (p *Project) GetWorkingDir(dir string) string {
	// Highest preference is for directories passed into the command directly
	if dir != "" {
		return dir
	}

	// The next highest preference is for directories defined in config.yaml
	if p.WorkingDir != "" {
		return p.WorkingDir
	}

	// The next highest preference is for app type defaults
	return "/var/www/html/"
}

// GetRelativeDirectory returns the directory relative to project root
// Note that the relative dir is returned as unix-style forward-slashes
func (p *Project) GetRelativeDirectory(targetPath string) string {
	// Find the relative dir
	relativeWorkingDir := strings.TrimPrefix(targetPath, p.AppRoot)
	// Convert to slash/linux/macos notation, should work everywhere
	relativeWorkingDir = filepath.ToSlash(relativeWorkingDir)
	// Remove any leading /
	relativeWorkingDir = strings.TrimLeft(relativeWorkingDir, "/")

	return relativeWorkingDir
}

// GetRelativeWorkingDirectory returns the relative working directory relative to project root
// Note that the relative dir is returned as unix-style forward-slashes
func (p *Project) GetRelativeWorkingDirectory() string {
	pwd, _ := os.Getwd()
	return p.GetRelativeDirectory(pwd)
}

// SiteStatus returns the current status of a project determined from web and db service health.
// returns status, statusDescription
// Can return SiteConfigMissing, SiteDirMissing, SiteStopped, SiteStarting, SiteRunning, SitePaused,
// or another status returned from dockerutil.GetContainerHealth(), including
// "exited", "restarting", "healthy"
func (p *Project) SiteStatus() string {
	if p.Status != "" {
		return p.Status
	}

	if !fileutil.FileExists(p.AppRoot) {
		p.Status = SiteDirMissing
		return p.Status
	}
	if _, err := IsProjectExists(p.AppRoot); err != nil {
		p.Status = SiteConfigMissing
		return p.Status
	}

	c, err := FindContainerByType(nodeps.WebContainer, p.Name)
	if err != nil {
		return ""
	}
	if c == nil {
		p.Status = SiteStopped
		return p.Status
	} else {
		status, _ := dockerutil.GetContainerHealth(c)
		switch {
		case status == "exited":
			p.Status = SiteStopped
		case status == "starting":
			p.Status = SiteStarting
		case status == "healthy":
			p.Status = SiteRunning
		default:
			p.Status = status
		}
	}

	return p.Status
}

// Start starts the project by running the docker compose up command
func (p *Project) Start(profiles ...string) (err error) {
	if status := p.SiteStatus(); status == SiteRunning && !p.NoCache {
		return
	}

	p.ComposeYAML = nil
	portToCheck := []*string{&LodevConfig.HttpPort, &LodevConfig.HttpsPort}
	GetEphemeralPortsIfNeeded(portToCheck, true)
	_ = p.DockerEnv()

	dockerutil.RemoveNetworkDuplicate(p.GetNetworkName())

	if err := CheckDockerComposeVersion(); err != nil {
		if os.IsTimeout(err) || strings.Contains(err.Error(), "timeout") {
			util.WarningMessage(fmt.Sprintf("Failed to download updated docker-compose binary.\nThis might be due to networks issues or Github being down. Please ensure your network stable and try again: %v", err))
		} else {
			util.WarningMessage(fmt.Sprintf("LODEV's private docker-compose binary does not exist or is set to an invalid version. Please use a docker-compose system by command 'lodev config global --required-docker-compose-version=\"\" --use-docker-compose-from-path=false': %v", err))
		}
	}

	p.CheckExistingProjectRegistry()

	yamSpinner := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	// WriteConfig .lodev-docker-compose-*.yaml
	yamSpinner.Start("Generating compose YAML")
	err = p.WriteDockerComposeYAML()
	yamSpinner.Stop("Compose YAML generated", 0)

	// This needs to be done after WriteDockerComposeYAML() to get the right images
	additionalImages, err := p.FindAllImages()
	if err != nil {
		return err
	}

	var pullWg sync.WaitGroup
	wait := util.StartWait(fmt.Sprintf("Waiting pulled images: %v", additionalImages))
	pullWg.Go(func() {
		if pullErr := PullBaseContainerImages(additionalImages, p.NoCache, wait.Progress); pullErr != nil {
			util.Warning("Unable to pull Docker images: %v", pullErr)
		}
	})

	if err := p.WriteProjectConfig(); err != nil {
		return err
	}

	if err := p.GenerateWebserverConfig(); err != nil {
		return err
	}

	// Wait for background image pull to finish before fingerprinting, so
	// ImageID() reflects the freshly-pulled digest rather than a stale local one.
	pullWg.Wait()
	wait.Complete(nil, "Images pulled successfully.")

	// Build extra layers on web and db images if necessary.
	// Skip the build entirely if the build context hasn't changed and built images exist.
	buildHashFile := p.GetConfigPath(".build-hash")
	currentBuildHash := nodeps.LodevFileSignature + "\n" + p.buildContextFingerprint()
	savedBuildHash, _ := os.ReadFile(buildHashFile)
	buildNeeded := p.NoCache || currentBuildHash != string(savedBuildHash)

	if !buildNeeded {
		// Verify the built images still exist locally
		webBuilt := nodeps.WebImage + "-" + p.Name + "-built"
		buildNeeded, _ = dockerutil.ImageExistsLocally(webBuilt)
	}
	if buildNeeded {
		_, err = p.composeBuild()
		if err != nil {
			return err
		}

		// Save build hash on successful build
		_ = os.WriteFile(buildHashFile, []byte(currentBuildHash), 0644)
	} else {
		util.Debug("Skipping docker-compose build, build context unchanged and images exist")
	}

	// Wait for web containers to become healthy
	dependers := []string{"web"}
	startSping := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	startSping.Start(fmt.Sprintf("Starting %v services...", dependers))
	util.Debug("Executing docker-compose -f %s up -d", p.DockerComposeFullRenderedYAMLPath())
	_, err = ComposeCmd(&ComposeCmdOpts{
		ComposeFiles: []string{p.DockerComposeFullRenderedYAMLPath()},
		Profiles:     profiles,
		Action:       []string{"up", "-d"},
	})
	if err != nil {
		return err
	}

	if !IsRouterDisabled() {
		caRoot := GetCAROOT()
		if caRoot == "" {
			util.WarningMessage("mkcert may not be properly installed, we suggest installing it for trusted https support, `brew install mkcert nss`, `choco install -y mkcert`, etc. and then `mkcert -install`")
		} else {
			if err := fileutil.CopyDir(caRoot, GetLodevConfigPath("traefik", "mkcert"), true); err != nil {
				return fmt.Errorf("failed to copy mkcert certs: %v", err)
			}
		}

		// If TLS supported and using Traefik, create cert/key in project's .lodev/traefik
		// The actual push to lodev-global-cache happens in PushGlobalTraefikConfig
		err = configurateTraefikForProject(p)
		if err != nil {
			return err
		}
	}

	// At this point we should have all files synced inside the container
	util.Debug("Running /start.sh in lodev-webserver")
	stdout, err := p.Exec(&ExecOpts{
		// Send output to /var/tmp/logpipe to get it to docker logs
		// If start.sh dies, we want to make sure the container gets killed off
		// so send SIGTERM to process ID 1
		Cmd:    `/start.sh > /var/tmp/logpipe 2>&1 || kill -- -1`,
		Detach: true,
	})
	if err != nil {
		startSping.Stop("Services failed to start.", 2)
		util.Warning("Unable to run /start.sh, stdout=%s: %v", stdout, err)
	} else {
		startSping.Stop(fmt.Sprintf("Started %v services.", dependers), 0)
	}

	waitingSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	waitingSpin.Start(fmt.Sprintf("Waiting for containers to become ready: %v", dependers))
	waitErr := p.Wait(dependers)
	waitingSpin.Stop(fmt.Sprintf("Containers %v are ready", dependers), 0)

	if waitErr != nil {
		return waitErr
	}

	if len(p.WebExtraDaemons) > 0 {
		extraDaemonSpin := tap.NewSpinner(tap.SpinnerOptions{})
		extraDaemonSpin.Start("Starting web extra daemons...")
		stdout, err := p.Exec(&ExecOpts{
			Cmd: `supervisorctl start webextradaemons:*`,
		})
		if err != nil {
			util.Warning("Unable to start web_extra_daemons using supervisorctl, stdout=%s, %v", stdout, err)
		}
		extraDaemonSpin.Stop("Web extra daemons started.", 0)
	}

	type execResult struct {
		serviceErr error
		routerErr  error
		waitErr2   error
	}
	result := execResult{}
	var wg sync.WaitGroup

	wg.Go(func() {
		if !IsRouterDisabled() {
			if len(LodevConfig.ConnectedServices) > 0 {
				result.serviceErr = StartLodevService(false, false)
			}
			result.routerErr = StartLodevRouter(false)
		}

		if result.serviceErr != nil || result.routerErr != nil {
			return
		}

		waitLabels := map[string]string{
			dockerutil.LabelAppName:     p.GetName(),
			"com.docker.compose.oneoff": "False",
		}
		containersAwaited, findErr := dockerutil.FindContainersByLabels(waitLabels)
		if findErr != nil {
			result.waitErr2 = findErr
			return
		}
		containerNames := dockerutil.GetContainerNames(containersAwaited, []string{}, "lodev-"+p.Name+"-")

		if len(containerNames) > 0 {
			waitingSpin = tap.NewSpinner(tap.SpinnerOptions{})
			waitingSpin.Start(fmt.Sprintf("Waiting for additional project containers %v to become ready", containerNames))
			result.waitErr2 = p.WaitByLabels(waitLabels)
			if result.waitErr2 != nil {
				waitingSpin.Stop(fmt.Sprintf("Failed to wait for containers %v", containerNames), 2, tap.StopOptions{
					Hint: fmt.Sprintf("Error: %v", result.waitErr2),
				})
			} else {
				waitingSpin.Stop(fmt.Sprintf("Additional containers %v are ready", containerNames), 0)
			}
		}
	})

	wg.Wait()

	p.Status = SiteRunning

	LodevConfig.LastStartedVersion = nodeps.LodevVersion
	_ = SaveLodevConfig()

	return
}

// StartAppIfNotRunning is intended to replace much-duplicated code in the commands.
func (p *Project) StartAppIfNotRunning() error {
	var err error
	status := p.SiteStatus()
	if status != SiteRunning {
		err = p.Start()
	}

	return err
}

// Stop stops the project by running the docker compose down command and optionally removing data such as certs, hosts entries, and built images
func (p *Project) Stop(removeData bool) error {
	_ = p.DockerEnv()
	if p.Name == "" {
		return fmt.Errorf("invalid project.Name provided to project.Stop(), project=%v", p)
	}
	status := p.SiteStatus()
	if status == SiteStopped {
		util.Debug("Project %s is already stopped", p.Name)
		return nil
	}
	// Remove container/network resources
	stopSping := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	stopSping.Start(fmt.Sprintf("Stopping project %s...", p.Name))
	if fileutil.FileExists(p.DockerComposeFullRenderedYAMLPath()) {
		_, err := ComposeCmd(&ComposeCmdOpts{
			ComposeFiles: []string{p.DockerComposeFullRenderedYAMLPath()},
			Profiles:     []string{`*`},
			Action:       []string{"down"},
		})
		if err != nil {
			util.WarningMessage("Failed to docker-compose down", tap.MessageOptions{
				Hint: fmt.Sprintf("Error: %v", err),
			})
		}
		labels := dockerutil.GetDockerLodevLabels(p.Name, nil)
		if err = dockerutil.RemoveContainerByLabels(labels); err != nil {
			util.WarningMessage("Failed to remove containers", tap.MessageOptions{
				Hint: fmt.Sprintf("Error: %v", err),
			})
		}
	}

	stopSping.Stop(fmt.Sprintf("Project %s stopped.", p.Name), 0)

	if removeData {
		// remove project cert name
		st := tap.NewStream(tap.StreamOptions{ShowTimer: true})
		st.Start("Removing project cert file")
		sourceCertDir := GetLodevConfigPath("traefik", "certs")
		sourceConfigDir := GetLodevConfigPath("traefik", "config")
		st.WriteLine("Remove project certifcates file")
		var certRemoveErr error
		for _, f := range []string{p.Name + ".crt", p.Name + ".key"} {
			if certRemoveErr = os.Remove(filepath.Join(sourceCertDir, f)); certRemoveErr != nil && !os.IsNotExist(certRemoveErr) {
				st.WriteLine(fmt.Sprintf("Failed to remove cert file: %v", certRemoveErr))
			}
		}

		if certRemoveErr == nil {
			st.WriteLine("Removed project cert files")
		}

		st.WriteLine("Removing Traefik configuration")
		traefikConfigFile := p.Name + "_config.yaml"
		if err := os.Remove(filepath.Join(sourceConfigDir, traefikConfigFile)); err != nil && !os.IsNotExist(err) {
			st.WriteLine(fmt.Sprintf("Failed to remove config file (%s): %v", traefikConfigFile, err))
		} else {
			st.WriteLine("Removed Traefik configuration")
		}

		if err := hostname.RemoveHostsIfNeeded(p.GetHostnames()); err != nil {
			st.WriteLine(fmt.Sprintf("Failed to remove hosts entry for project: %v", err))
		} else {
			st.WriteLine("Removed hosts entry for project")
		}

		st.WriteLine("Removing project image built")
		RemoveProjectRegistry(p.Name)
		webBuilt := p.WebImage + "-" + p.Name + "-built"
		_ = dockerutil.RemoveImage(webBuilt)
		_ = os.RemoveAll(p.GetConfigPath())
		st.WriteLine(fmt.Sprintf("Removed project image built: %s", webBuilt))
	}

	p.Status = SiteStopped

	// clear(Ele)
	return nil
}

// Wait ensures that the app service containers are healthy.
// All requested containers are waited on in parallel using a single polling loop.
func (p *Project) Wait(requiredContainers []string) error {
	labels := map[string]string{
		dockerutil.LabelAppName:     p.GetName(),
		"com.docker.compose.oneoff": "False",
	}
	waitTime := p.GetMaxContainerWaitTime()
	return dockerutil.ContainersWait(waitTime, labels, requiredContainers...)
}

// WaitByLabels waits for containers found by list of labels to be
// ready
func (p *Project) WaitByLabels(labels map[string]string) error {
	waitTime := p.GetMaxContainerWaitTime()
	err := dockerutil.ContainersWait(waitTime, labels)
	if err != nil {
		return fmt.Errorf("container(s) failed to become healthy before their configured timeout or in %d seconds.\nThis might be a problem with the healthcheck and not a functional problem.\nThe error was '%v'", waitTime, err.Error())
	}
	return nil
}

// GetMaxContainerWaitTime looks through all services and returns the max time we expect
// to wait for all containers to become `healthy`. Mostly this is healthcheck.start_period.
// Defaults to DefaultContainerTimeout (usually 120 unless overridden)
func (p *Project) GetMaxContainerWaitTime() int {
	defaultContainerTimeout := 3000
	maxWaitTime := defaultContainerTimeout

	if p.ComposeYAML == nil || p.ComposeYAML.Services == nil {
		return defaultContainerTimeout
	}
	for _, service := range p.ComposeYAML.Services {
		if service.HealthCheck == nil {
			continue
		}
		if service.HealthCheck.StartPeriod != nil {
			duration, err := time.ParseDuration(service.HealthCheck.StartPeriod.String())
			if err != nil {
				continue
			}
			t := int(duration.Seconds())
			if t > maxWaitTime {
				maxWaitTime = t
			}
			continue
		}
		// In this case we didn't have a specified start_period, so guess at one
		// Use defaults for interval and retries
		// https://docs.docker.com/reference/dockerfile/#healthcheck
		interval := 5
		retries := 3

		if service.HealthCheck.Interval != nil {
			intervalInt, err := time.ParseDuration(service.HealthCheck.Interval.String())
			if err == nil {
				interval = int(intervalInt.Seconds())
			}
		}
		if service.HealthCheck.Retries != nil {
			retries = int(*service.HealthCheck.Retries)
		}
		// If the retries*interval is greater than what we've found before
		// then use it. This will be unusual.
		if retries*interval > maxWaitTime {
			maxWaitTime = retries * interval
		}
	}
	return maxWaitTime
}

// composeBuild executes docker-compose build with retry logic for BuildKit snapshot race conditions.
// This is an experimental workaround for moby/buildkit#6521 (Docker 29+ with containerd image store).
// The race condition causes intermittent failures with "parent snapshot ... does not exist: not found"
// when multiple services share base layers and build in parallel.
//
// args are optional extra arguments to pass to the build command (e.g., service name, "--no-cache")
// Returns the stdout output on success, or an error if all retries are exhausted.
func (p *Project) composeBuild(args ...string) (string, error) {
	wait := util.StartWait("Building images...")
	progress := "plain"
	composeBuildMaxRetries := 3

	action := []string{"--progress=" + progress, "build"}
	if p.NoCache {
		action = append(action, "--no-cache")
	}
	action = append(action, args...)

	var lastErr error
	var out string

	for attempt := 1; attempt <= composeBuildMaxRetries; attempt++ {
		util.Debug("Executing docker-compose -f %s %s (attempt %d/%d)", p.DockerComposeFullRenderedYAMLPath(), strings.Join(action, " "), attempt, composeBuildMaxRetries)

		buf, lastErr := ComposeCmd(&ComposeCmdOpts{
			ComposeFiles: []string{p.DockerComposeFullRenderedYAMLPath()},
			Action:       action,
			Progress:     wait.Progress,
			Timeout:      time.Hour * 1,
		})
		out = buf.String()

		if lastErr == nil {
			wait.Complete(lastErr, "Images built successfully.")
			util.Debug("docker-compose build output:\n%s\n\n", buf.String())
			return buf.String(), nil
		}

		// Check if this is the known BuildKit snapshot race condition
		errorText := fmt.Sprintf("%v %s", lastErr, buf.String())
		isSnapshotRace := strings.Contains(errorText, "parent snapshot") && strings.Contains(errorText, "does not exist")

		if !isSnapshotRace {
			lastErr = fmt.Errorf("docker-compose build failed with error: %v, output: %s", lastErr, buf.String())
			wait.Complete(lastErr)
			// Not a snapshot race error, fail immediately without retry
			return buf.String(), lastErr
		}

		// This is a snapshot race error - retry if we have attempts remaining
		if attempt < composeBuildMaxRetries {
			util.Warning("BuildKit snapshot race condition detected (moby/buildkit#6521). Retrying build (attempt %d/%d)...", attempt+1, composeBuildMaxRetries)
		}
	}

	// All retries exhausted
	lastErr = fmt.Errorf("docker-compose build failed after %d attempts: %v, output='%s', stderr='%s'", composeBuildMaxRetries, lastErr, out, "")
	wait.Complete(lastErr)
	return out, lastErr
}

// buildContextFingerprint returns an SHA-256 hash of the build context directories
// (.webimageBuild) and base image IDs. This is used to detect
// when docker-compose build can be skipped because nothing has changed.
// Image IDs (sha256 digests) are used instead of tag names so that
// rebuilt images with the same tag are still detected as changed.
func (p *Project) buildContextFingerprint() string {
	hash, err := fileutil.HashDirs([]string{p.GetConfigPath(".webimageBuild"), dockerutil.ImageID(p.WebImage)})
	if err != nil {
		util.Warning("unable to hash build context directories: %v", err)
		return ""
	}
	return hash
}

// SetCommonEnv sets the common environment variables for the project
func SetCommonEnv(composeName string) map[string]string {
	uidStr, gidStr, username := dockerutil.GetContainerUser()
	// Warn about running as root if we're not on Windows.
	if uidStr == "0" || gidStr == "0" {
		util.Warning("Warning: containers will run as root. This could be a security risk on Linux.")
	}

	lodevDir := fileutil.WindowsPathToCygwinPath(GetLodevConfigDir())
	lodevData := os.Getenv("LODEV_DATA")
	if lodevData == "" {
		lodevData = fileutil.WindowsPathToCygwinPath(GetLodevConfigPath("data"))
	}

	if !fileutil.FileExists(lodevData) {
		os.MkdirAll(lodevData, 0755)
	}

	envVars := map[string]string{
		"COMPOSE_PROJECT_NAME":           composeName,
		"COMPOSE_REMOVE_ORPHANS":         "true",
		"COMPOSER_EXIT_ON_PATCH_FAILURE": "1",
		"LODEV_UID":                      uidStr,
		"LODEV_GID":                      gidStr,
		"LODEV_USER":                     username,
		"LODEV_TLD":                      LodevConfig.ProjectTld,
		"LODEV_DEFAULT":                  lodevDir,
		"LODEV_DATA":                     lodevData,
		"LODEV_CONFIG":                   lodevDir,
		"LODEV_GOOS":                     runtime.GOOS,
		"LODEV_GOARCH":                   runtime.GOARCH,
		"LODEV_VERSION":                  nodeps.LodevVersion,
		"DOCKER_SCAN_SUGGEST":            "false",
		"IS_LODEV_PROJECT":               "true",
		"IS_DEVCONTAINER":                strconv.FormatBool(util.IsDevcontainer()),
	}

	// Only set values if they don't already exist in env.
	for k, v := range envVars {
		if err := os.Setenv(k, v); err != nil {
			util.ErrorMessage(fmt.Sprintf("Failed to set the environment variable %s=%s: %v", k, v, err))
		}
	}
	return envVars
}

// CheckExistingProjectRegistry looks to see if we already have a project in this approot with different name
func (p *Project) CheckExistingProjectRegistry() error {
	for name, v := range LodevProjectsRegistry {
		if p.AppRoot == v.AppRoot && name != p.Name {
			return fmt.Errorf(`this project root '%s' already contains a project named '%s'. You may want to remove the existing project with "lodev stop --unlist %s"`, v.AppRoot, name, name)
		}
	}

	CopyIntoProjectAssets(p.Name)
	return nil
}

// DockerEnv sets environment variables for a docker-compose run.
func (p *Project) DockerEnv() map[string]string {
	envVars := SetCommonEnv(p.GetComposeProjectName())
	if util.IsDevcontainer() {
		if p.HostWebserverPort == "" {
			p.HostWebserverPort = "8080"
		}
		if p.HostHttpsPort == "" {
			p.HostHttpsPort = "8443"
		}
	}
	// Figure out what the host-webserver (host-http) port is
	// First we try to see if there's an existing webserver container and use that
	hostHTTPPort, err := p.GetPublishedPort("web")
	hostHTTPPortStr := ""
	// Otherwise we'll use the configured value from app.HostWebserverPort
	if hostHTTPPort > 0 && err == nil {
		hostHTTPPortStr = strconv.Itoa(hostHTTPPort)
	} else if p.HostWebserverPort != "" {
		hostHTTPPortStr = p.HostWebserverPort
	}

	// Figure out what the host-webserver https port is
	// the https port is rarely used because lodev-router does termination
	// for the vast majority of applications
	hostHTTPSPort, err := p.GetPublishedPortForPrivatePort("web", 443)
	hostHTTPSPortStr := ""
	if hostHTTPSPort > 0 && err == nil {
		hostHTTPSPortStr = strconv.Itoa(hostHTTPSPort)
	} else {
		hostHTTPSPortStr = p.HostHttpsPort
	}

	newEnvVars := map[string]string{
		// The compose project name can no longer contain dots; must be lower-case
		"COMPOSE_PROJECT_NAME":    p.GetComposeProjectName(),
		"LODEV_APPNAME":           p.Name,
		"LODEV_TLD":               LodevConfig.ProjectTld,
		"LODEV_PROJECT":           p.Name,
		"LODEV_WEBIMAGE":          p.WebImage,
		"LODEV_APPROOT":           p.AppRoot,
		"LODEV_COMPOSER_ROOT":     p.GetComposerRoot(true, false),
		"LODEV_HOST_HTTP_PORT":    hostHTTPPortStr,
		"LODEV_HOST_HTTPS_PORT":   hostHTTPSPortStr,
		"LODEV_DOCROOT":           p.GetDocroot(),
		"LODEV_HOSTNAME":          p.GetHostname(),
		"LODEV_PHP_VERSION":       p.PHPVersion,
		"LODEV_WEBSERVER":         p.Webserver,
		"LODEV_PROJECT_TYPE":      p.Type,
		"LODEV_ROUTER_HTTP_PORT":  LodevConfig.HttpPort,
		"LODEV_ROUTER_HTTPS_PORT": LodevConfig.HttpsPort,
		"LODEV_XDEBUG_ENABLED":    strconv.FormatBool(p.XdebugEnabled),
		"LODEV_PRIMARY_URL":       p.GetPrimaryURL(),
	}

	maps.Copy(newEnvVars, envVars)
	for k, v := range newEnvVars {
		if err := os.Setenv(k, v); err != nil {
			util.ErrorMessage(fmt.Sprintf("Failed to set the environment variable %s=%s: %v", k, v, err))
		}
	}
	return newEnvVars
}

// injectLabelsComposeYAML stamps LODEV labels onto all services, build sections, and
// non-external networks in the projects
func injectLabelsComposeYAML(project *composeTypes.Project, labels map[string]string) {
	for name, service := range project.Services {
		if service.Labels == nil {
			service.Labels = composeTypes.Labels{}
		}
		maps.Copy(service.Labels, labels)
		if service.Build != nil {
			if service.Build.Labels == nil {
				service.Build.Labels = composeTypes.Labels{}
			}
			maps.Copy(service.Build.Labels, labels)
		}
		project.Services[name] = service
	}
	for name, network := range project.Networks {
		if network.External {
			continue
		}
		if network.Labels == nil {
			network.Labels = composeTypes.Labels{}
		}
		maps.Copy(network.Labels, labels)
		project.Networks[name] = network
	}
}

// EnsureLodevNetwork creates or ensures the LODEV network exists or exits with fatal.
func EnsureLodevNetwork() error {
	// Ensure we have the fallback global LODEV network
	netOptions := client.NetworkCreateOptions{
		Driver:   "bridge",
		Internal: false,
		Labels:   dockerutil.GetDefaultDockerLodevLabels(),
	}
	err := dockerutil.EnsureNetwork(nodeps.LodevNetwork, netOptions)
	if err != nil {
		return fmt.Errorf("Failed to ensure Docker network %s: %v", nodeps.LodevNetwork, err)
	}

	return nil
}

// ComposeFiles returns a list of compose files for a project.
// It has to put the .lodev/docker-compose.*.y*ml first
// It has to put the .lodev/docker-compose.override.y*ml last
func ComposeFiles(src string, mainComposeFile string, composeFileGlob string) ([]string, error) {
	origDir, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err := os.Chdir(src)
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(src, composeFileGlob))
	if err != nil {
		return []string{}, fmt.Errorf("unable to glob %s in %s: err=%v", composeFileGlob, src, err)
	}

	if !fileutil.FileExists(mainComposeFile) {
		return nil, fmt.Errorf("failed to find %s", mainComposeFile)
	}

	overrides, err := filepath.Glob(filepath.Join(src, "docker-compose.override.y*ml"))
	if err != nil {
		util.Failed("FAILED: %s", err)
	}

	orderedFiles := make([]string, 1)

	// Make sure the main file goes first
	orderedFiles[0] = mainComposeFile

	for _, file := range files {
		// We already have the main file, and it's not in the list anyway, so skip when we hit it.
		// We'll add the override later, so skip it.
		if len(overrides) == 1 && file == overrides[0] {
			continue
		}
		orderedFiles = append(orderedFiles, filepath.Join(src, file))
	}
	if len(overrides) == 1 {
		orderedFiles = append(orderedFiles, filepath.Join(src, overrides[0]))
	}
	return orderedFiles, nil
}

// EnvFiles returns a list of env files for a project.
// It has to put the .lodev/.env first
// It has to put the .lodev/.env.* second
// Env files ending with .example are ignored.
func EnvFiles(src string) ([]string, error) {
	origDir, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(origDir)
	}()
	err := os.Chdir(src)
	if err != nil {
		return nil, err
	}
	envFiles, err := filepath.Glob(filepath.Join(src, ".env.*"))
	if err != nil {
		return []string{}, fmt.Errorf(".env.* in %s: err=%v", src, err)
	}
	var orderedEnvFiles []string
	defaultEnvFile := filepath.Join(src, ".env")
	if fileutil.FileExists(defaultEnvFile) {
		orderedEnvFiles = append(orderedEnvFiles, defaultEnvFile)
	}
	for _, file := range envFiles {
		// Skip .example files
		if strings.HasSuffix(file, ".example") {
			continue
		}
		orderedEnvFiles = append(orderedEnvFiles, filepath.Join(src, file))
	}
	return orderedEnvFiles, nil
}

// ReadEnvFile reads the .env file into a envText and envMap
// The map has the envFile content, but without comments
// returns
// - envMap (map of items found)
// - envText (plain text unaltered of existing env file
// - error/nil
func ReadEnvFile(envFilePath string) (envMap map[string]string, envText string, err error) {
	// envFilePath := filepath.Join(app.AppRoot, ".env")
	envText, _ = fileutil.ReadFileIntoString(envFilePath)
	// godotenv is not perfect, there can be some edge cases with escaping
	// such as https://github.com/joho/godotenv/issues/225
	envMap, err = godotenv.Read(envFilePath)

	return envMap, envText, err
}
