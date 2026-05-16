package lodev

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/types/container"
	"github.com/yarlson/tap"

	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/otiai10/copy"
	"go.yaml.in/yaml/v4"
)

// Service represents a single LODEV service from the registry
type Service struct {
	Name             string             `yaml:"name"`
	Description      string             `yaml:"description"`
	VersionContraint string             `yaml:"version_constraint"`
	ComposeFiles     []string           `yaml:"compose_files"`
	GlobalFiles      []string           `yaml:"global_files"`
	ProjectFiles     []string           `yaml:"project_files"`
	Type             string             `yaml:"type"`
	ConfigFile       string             `yaml:"config_file,omitempty"`
	SourcePath       string             `yaml:"source_path,omitempty"`
	Container        *container.Summary `yaml:"-"`
}

// ServiceList represents the complete services registry data structure
type ServiceList struct {
	TotalService int                   `yaml:"total_service"`
	ForceRefresh bool                  `yaml:"force_refresh"`
	Services     []*Service            `yaml:"services"`
	ComposeYAML  *composeTypes.Project `yaml:"-"`
}

// LodevServices is the global variable that holds the cached list of services from the registry.
var LodevServices ServiceList

// FindService searches for a service in the ServiceList by its name (e.g., "lodev-redis")
func (sl *ServiceList) FindService(name string) *Service {
	for _, service := range sl.Services {
		if service.Name == name {
			return service
		}
	}
	return nil
}

// ServiceComposeFullPath returns the absolute path to where the
// complete generated yaml file should exist for those services.
func (sl *ServiceList) ServiceComposeFullPath() string {
	return GetLodevConfigPath(".service-compose-full.yaml")
}

// ServiceComposeYAMLPath returns the absolute path to where
// the base generated yaml file should exist for those services.
func (sl *ServiceList) ServiceComposeYAMLPath() string {
	return GetLodevConfigPath(".service-compose.yaml")
}

// WriteDockerComposeYAML writes a .service-compose.yaml and related to the ~/.lodev directory.
// It then uses `docker-compose convert` to get a canonical version of the full compose file.
// It then makes a couple of fixups to the canonical version (networks and approot bind points) by
// marshaling the canonical version to YAML and then unmarshaling it back into a canonical version.
func (sl *ServiceList) WriteDockerComposeYAML() error {
	var composeFiles []string
	SetCommonEnv(nodeps.ServiceComposeProjectName)
	for _, s := range LodevConfig.ConnectedServices {
		service := sl.FindService(s)
		if service == nil {
			continue
		}
		for _, cf := range service.ComposeFiles {
			composeFilePath := filepath.Join(service.SourcePath, cf)
			if !fileutil.FileExists(composeFilePath) {
				continue
			}
			composeFiles = append(composeFiles, composeFilePath)
		}
	}

	if len(composeFiles) == 0 {
		util.ErrorMessage("No service enabled.")
	}
	serviceYaml, err := util.MergeYAML(composeFiles[0], composeFiles[1:]...)

	if err = os.WriteFile(sl.ServiceComposeYAMLPath(), []byte(serviceYaml), 0755); err != nil {
		return err
	}

	envFiles, err := EnvFiles(GetLodevConfigDir())

	if err != nil {
		return err
	}

	var action []string

	for _, envFile := range envFiles {
		action = append(action, "--env-file", envFile)
	}

	userFiles, err := filepath.Glob(GetLodevConfigPath(".service-compose.*.y*ml"))
	if err != nil {
		return err
	}
	files := append([]string{sl.ServiceComposeYAMLPath()}, userFiles...)

	fullContent, err := ComposeCmd(&ComposeCmdOpts{
		ComposeFiles: files,
		Profiles:     []string{`*`},
		Action:       append(action, "config"),
	})

	if err != nil {
		return fmt.Errorf("Failed to render docker compose YAML: %v", err)
	}

	sl.ComposeYAML, err = sl.EnsureComposeYAML(fullContent.String())

	fullContentsBytes, err := sl.ComposeYAML.MarshalYAML()
	if err != nil {
		return err
	}

	fullContentsBytes = util.EscapeDollarSign(fullContentsBytes)
	fullPath := sl.ServiceComposeFullPath()
	_ = os.Remove(fullPath)
	f, _ := os.Create(fullPath)

	defer f.Close()

	_, err = f.Write(fullContentsBytes)
	if err != nil {
		return err
	}

	return nil
}

// EnsureComposeYAML takes the rendered docker compose YAML content and ensures that it has the required LODEV network and labels injected
// then returns the parsed compose project struct
func (sl *ServiceList) EnsureComposeYAML(yamlStr string) (*composeTypes.Project, error) {
	project, err := EnsureComposeYAML(yamlStr)
	if err != nil {
		return project, err
	}
	project.Name = nodeps.ServiceComposeProjectName
	// Ensure that some important network properties are not overridden by users
	if _, ok := project.Networks[nodeps.LodevNetwork]; !ok {
		project.Networks[nodeps.LodevNetwork] = composeTypes.NetworkConfig{}
	}
	if _, ok := project.Networks["default"]; !ok {
		project.Networks["default"] = composeTypes.NetworkConfig{}
	}
	for name, network := range project.Networks {
		if nodeps.LodevNetwork == name {
			network.Name = nodeps.LodevNetwork
			network.External = true
		} else if name == "default" {
			network.Name = nodeps.LodevServiceNetwork
			network.External = false
		}
		project.Networks[name] = network
	}
	envFiles, err := EnvFiles(GetLodevConfigDir())
	if err != nil {
		return project, err
	}

	labels := dockerutil.GetDockerLodevLabels(nodeps.ServiceComposeProjectName, map[string]string{
		dockerutil.LabelType: dockerutil.LabelTypeService,
	})

	injectLabelsComposeYAML(project, labels)

	// Ensure all services have required networks and environment variables
	for name, service := range project.Services {
		if _, ok := service.Networks[nodeps.LodevNetwork]; !ok {
			service.Networks[nodeps.LodevNetwork] = nil
		}

		if service.Labels == nil {
			service.Labels = composeTypes.Labels{}
		}

		// Add environment variables from .env files to services
		for _, envFile := range envFiles {
			filename := filepath.Base(envFile)
			// Variables from .lodev/.env should be available in all containers,
			// and variables from .lodev/.env.* should only be available in a specific container.
			if filename == ".env" || filename == ".env."+name {
				envMap, _, err := ReadEnvFile(envFile)
				if err != nil && !os.IsNotExist(err) {
					util.Failed("Unable to read %s file: %v", envFile, err)
				}
				for envKey, envValue := range envMap {
					val := envValue
					service.Environment[envKey] = &val
				}
			}
		}
	}

	return project, err
}

// CheckVersionConstraint checks if the current LODEV version meets the service's version constraint, if specified
func (s *Service) CheckVersionConstraint() error {
	if s.VersionContraint == "" {
		return nil
	}
	return CheckLodevVersionConstraint(s.VersionContraint, fmt.Sprintf("Service %s is not compatible with your LODEV version", s.Name), "")
}

// ExpandFilesAndDirectories extracts the service files
func (s *Service) ExpandFilesAndDirectories(appName string) error {
	copyFileFunc := func(src string, dest string) (err error) {
		util.Debug("Extracting project file for add-on '%s': %s -> %s", s.Name, src, dest)
		destDir := filepath.Dir(dest)
		if err = os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("Unable to create directory %s: %v", destDir, err)
		}
		if err = fileutil.CheckSignatureOrNoFile(dest, nodeps.LodevFileSignature); err == nil {
			err = copy.Copy(src, dest)
			if err != nil {
				return fmt.Errorf("Unable to copy %v to %v: %v", src, dest, err)
			}
		}

		return nil
	}

	extractGlobalFiles := func() error {
		lodevDir := GetLodevConfigDir()

		globalFiles, err := fileutil.ExpandFilesAndDirectories(s.SourcePath, s.GlobalFiles)
		if err != nil {
			return fmt.Errorf("Unable to expand files and directories: %v", err)
		}
		if len(globalFiles) == 0 {
			return nil
		}

		for _, file := range globalFiles {
			src := filepath.Join(s.SourcePath, file)
			dest := filepath.Join(lodevDir, file)
			if err = copyFileFunc(src, dest); err != nil {
				return err
			}
		}

		if err = os.Chdir(lodevDir); err != nil {
			return fmt.Errorf("Unable to change directory LODEV config global: %v", err)
		}

		return nil
	}

	if err := extractGlobalFiles(); err != nil {
		return err
	}

	return nil
}

// RemoveExpandedFiles removes the expanded files for a service.
// cleanup services files when removing a service
func (s *Service) RemoveExpandedFiles() error {
	lodevDir := GetLodevConfigDir()
	removeFileFunc := func(file string) error {
		err := os.Remove(file)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Unable to remove expanded file %s: %v", file, err)
		}
		return nil
	}
	for _, file := range s.GlobalFiles {
		dest := filepath.Join(lodevDir, file)
		_ = removeFileFunc(dest)
	}

	for _, pr := range LodevProjectsRegistry {
		prConfigDir := filepath.Join(pr.AppRoot, LodevDir)
		if !fileutil.FileExists(prConfigDir) {
			continue
		}
		for _, file := range s.GlobalFiles {
			dest := filepath.Join(prConfigDir, file)
			_ = removeFileFunc(dest)
		}
		for _, file := range s.ProjectFiles {
			dest := filepath.Join(prConfigDir, file)
			_ = removeFileFunc(dest)
		}
	}

	return nil
}

// GetContainer returns the Docker container associated with the service
func (s *Service) GetContainer() (*container.Summary, error) {
	if s.Container != nil {
		return s.Container, nil
	}
	query := map[string]string{
		dockerutil.LabelService: s.Name,
		dockerutil.LabelType:    dockerutil.LabelTypeService,
		dockerutil.LabelOneOff:  "False",
	}
	c, err := dockerutil.FindContainerByLabels(query)
	if err != nil {
		return nil, fmt.Errorf("Failed to find container for service %s: %v", s.Name, err)
	}
	s.Container = c
	return s.Container, nil
}

// GetServiceUrl returns the external URL(s) for the service by looking at the VIRTUAL_HOST environment variable of the service container
func (s *Service) GetServiceUrl() []string {
	c, err := s.GetContainer()
	if err != nil || c == nil {
		return []string{}
	}
	virtualHosts := dockerutil.GetContainerEnv("VIRTUAL_HOST", *c)
	if virtualHosts == "" {
		return []string{}
	}
	hostnames := strings.Split(virtualHosts, ",")
	externalHostnames := make([]string, 0, len(hostnames))

	for _, h := range hostnames {
		if !strings.Contains(h, LodevConfig.ProjectTld) {
			h = h + "." + LodevConfig.ProjectTld
		}
		h = strings.TrimSpace(h)
		externalHostnames = append(externalHostnames, h)
	}
	return externalHostnames
}

// GetStatus checks the status of the service by looking for a running container with the appropriate labels and checking its health status
func (s *Service) GetStatus() (string, string) {
	var status, logOutput string
	c, err := s.GetContainer()
	if err != nil || c == nil {
		status = SiteStopped
	} else {
		status, logOutput = dockerutil.GetContainerHealth(c)
	}

	return status, logOutput
}

// GetServiceList returns the list of available services from the registry, either from cache or by scanning the config directory
func GetServiceList(force ...bool) (*ServiceList, error) {
	forceRefresh := len(force) > 0 && force[0]
	if LodevServices.Services != nil && !forceRefresh {
		return &LodevServices, nil
	}

	configFile := GetLodevConfigPath("service_list.yaml")

	isGrepNeeded := forceRefresh || !fileutil.FileExists(configFile)

	if isGrepNeeded {
		services, _ := GrepServiceList()
		LodevServices = ServiceList{
			TotalService: len(services),
			Services:     services,
		}

		if err := SaveServiceList(); err != nil {
			util.WarningMessage(fmt.Sprintf("Warning: Unable to create service registry cache file: %v", err))
		}
		return &LodevServices, nil
	}

	err := util.LoadYAML(configFile, &LodevServices)

	return &LodevServices, err
}

// GetService retrieves a specific service by its name
func AddServices(force bool, name ...string) ([]string, error) {
	serviceList, err := GetServiceList(force)
	if err != nil {
		return []string{}, fmt.Errorf("Failed to get service list: %w", err)
	}

	addedServices := []string{}
	for _, n := range name {
		service := serviceList.FindService(n)
		if service == nil {
			util.WarningMessage(fmt.Sprintf("Service %s not found in registry. Skipping.", n))
			continue
		}
		if err := service.CheckVersionConstraint(); err != nil {
			util.WarningMessage(fmt.Sprintf("Service %s is not compatible with your LODEV version: %v. Skipping.", service.Name, err))
			continue
		}
		if slices.Contains(LodevConfig.ConnectedServices, service.Name) && !force {
			util.WarningMessage(fmt.Sprintf("Service %s is already connected. Skipping.", service.Name))
			continue
		}
		if err := service.ExpandFilesAndDirectories(""); err != nil {
			return addedServices, fmt.Errorf("%w", err)
		}
		addedServices = append(addedServices, service.Name)
		LodevConfig.ConnectedServices = append(LodevConfig.ConnectedServices, service.Name)
	}

	if err := SaveLodevConfig(); err != nil {
		return addedServices, fmt.Errorf("Failed to save Lodev config: %w", err)
	}

	if len(addedServices) == 0 && fileutil.FileExists(serviceList.ServiceComposeFullPath()) {
		return addedServices, nil
	}

	return addedServices, nil
}

// RemoveServices removes a specific service by its name from the connected services list in LodevConfig
func RemoveServices(force bool, name ...string) ([]string, error) {
	serviceList, err := GetServiceList(force)
	if err != nil {
		return []string{}, fmt.Errorf("Failed to get service list: %w", err)
	}

	removedServices := []string{}
	for _, n := range name {
		service := serviceList.FindService(n)
		if service == nil {
			util.WarningMessage(fmt.Sprintf("Service %s not found in registry. Skipping.", n))
			continue
		}
		if slices.Contains(LodevConfig.ConnectedServices, service.Name) || force {
			LodevConfig.ConnectedServices = slices.DeleteFunc(LodevConfig.ConnectedServices, func(e string) bool {
				return e == service.Name
			})
			if err := service.RemoveExpandedFiles(); err == nil {
				removedServices = append(removedServices, service.Name)
			}
			continue
		}
		util.WarningMessage(fmt.Sprintf("Service %s is not currently connected. Skipping.", service.Name))
	}

	if err := SaveLodevConfig(); err != nil {
		return []string{}, fmt.Errorf("Failed to save Lodev config: %w", err)
	}

	if len(removedServices) == 0 && fileutil.FileExists(serviceList.ServiceComposeFullPath()) {
		return []string{}, nil
	}

	return removedServices, nil
}

// SaveServiceList saves the current list of services in LodevServices to the config file (~/.lodev/service_list.yaml) for caching purposes
func SaveServiceList() error {
	configFile := GetLodevConfigPath("service_list.yaml")
	serviceBytes, err := yaml.Marshal(LodevServices)
	if err != nil {
		return err
	}

	if err = os.WriteFile(configFile, serviceBytes, 0644); err != nil {
		return err
	}

	return nil
}

// GrepServiceList scans the services config directory for all service config.yaml files, loads them, and returns a list of Service objects.
// This is used to populate the service registry cache.
func GrepServiceList() ([]*Service, error) {
	// If services are already loaded and no force refresh, return the cached services
	if LodevServices.Services != nil && !LodevServices.ForceRefresh {
		return LodevServices.Services, nil
	}

	serviceFiles, err := filepath.Glob(GetLodevServicePath("**", "config.yaml"))

	if err != nil {
		return nil, fmt.Errorf("failed to search for service config files: %w", err)
	}

	services := []*Service{}

	for _, sFile := range serviceFiles {
		var service Service
		if err := util.LoadYAML(sFile, &service); err != nil {
			util.Warning("Warning: failed to load service config from %s: %v\n", sFile, err)
			continue
		}
		service.ConfigFile = sFile
		service.SourcePath = filepath.Dir(sFile)
		services = append(services, &service)
	}
	LodevServices.Services = services

	return services, nil
}

// DescribeLodevServices returns a map which provides detailed information on services associated with the running site.
func DescribeLodevServices() [][]string {
	describes := [][]string{}
	serviceList, err := GetServiceList(true)
	if err != nil {
		return describes
	}
	userHome, _ := os.UserHomeDir()

	for _, service := range serviceList.Services {
		describe := make([]string, 0)
		describe = append(describe, service.Name)
		describe = append(describe, strings.Replace(service.SourcePath, userHome, "~", 1))
		describes = append(describes, describe)
	}

	return describes
}

// StartConnectedService starts a connected service that is not tied to a specific projects (e.g: lodev-mysql, lodev-mailpit, etc.)
// This can be used by multiple projects simultaneously.
func StartLodevService(startRouter bool, force bool) error {
	if err := EnsureLodevNetwork(); err != nil {
		return fmt.Errorf("Failed to ensure LODEV network (%s). ERROR: %v", nodeps.LodevNetwork, err)
	}
	_, err := GetServiceList(force)
	if err != nil {
		return err
	}

	if !force {
		services, err := FindLodevServices()
		if err == nil && len(services) > 0 {
			servicesRunning := []string{}
			for _, service := range services {
				serviceName, ok := service.Labels[dockerutil.LabelService]
				if ok && service.State == "running" {
					servicesRunning = append(servicesRunning, serviceName)
				}
			}
			allServicesRunning := true
			for _, s := range LodevConfig.ConnectedServices {
				if !slices.Contains(servicesRunning, s) {
					allServicesRunning = false
					break
				}
			}
			if allServicesRunning {
				return nil
			}
		}
	}
	SetCommonEnv(nodeps.ServiceComposeProjectName)
	if err := LodevServices.WriteDockerComposeYAML(); err != nil {
		return fmt.Errorf("Failed to write docker compose YAML: %w", err)
	}

	if force {
		if err := StopLodevService(); err != nil {
			return fmt.Errorf("Failed to stop services: %w", err)
		}
	}

	fullPath := LodevServices.ServiceComposeFullPath()
	if !fileutil.FileExists(fullPath) {
		return fmt.Errorf("Compose file not found at expected path: %s.\nPlease run 'lodev service --update' to refresh the service registry and generate the compose file.", fullPath)
	}

	util.Debug("Executing docker-compose -f %s up -d", fullPath)
	wait := util.StartWait("Start LODEV services")

	_, err = ComposeCmd(&ComposeCmdOpts{
		ComposeFiles: []string{fullPath},
		Profiles:     []string{`*`},
		Progress:     wait.Progress,
		Action:       []string{"up", "-d"},
	})

	wait.Complete(err, "Started LODEV Services")

	label := map[string]string{
		dockerutil.LabelType:        dockerutil.LabelTypeService,
		"com.docker.compose.oneoff": "False",
	}
	waitServicesSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	waitServicesSpin.Start(fmt.Sprintf("Waiting for services %v to become ready", LodevConfig.ConnectedServices))
	serviceWaitTimeout := 120
	util.Debug("Service wait: checking for container with labels %v, timeout %d seconds, polling every 500ms for healthy status", label, serviceWaitTimeout)
	logOutput, err := dockerutil.ContainerWait(serviceWaitTimeout, label)
	if err != nil {
		err = fmt.Errorf("LODEV services failed to start within %d seconds. ERROR: %v. Logs: %s", serviceWaitTimeout, err, logOutput)
		waitServicesSpin.Stop(err.Error(), 2)
		return err
	}
	waitServicesSpin.Stop(fmt.Sprintf("Services %v are ready", LodevConfig.ConnectedServices), 0)

	traefikSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	traefikSpin.Start("Configuring Traefik for services")
	if err := configurateTraefikForServices(&LodevServices); err != nil {
		err = fmt.Errorf("failed to configurate Traefik for services: %v", err)
		traefikSpin.Stop(err.Error(), 2)
		return err
	}
	traefikSpin.Stop("Pushed Traefik configuration", 0)

	os.Setenv("COMPOSE_PROJECT_NAME", nodeps.RouterComposeProjectName)
	if !startRouter {
		wait.Complete(err, "LODEV services started")
		return nil
	}

	if err := StartLodevRouter(false); err != nil {
		util.WarningMessage(fmt.Sprintf("Failed to start LODEV router: %v", err))
		return fmt.Errorf("Failed to start LODEV router: %v", err)
	}

	return nil
}

// StartConnectedService starts a connected service that is not tied to a specific projects (e.g: lodev-mysql, lodev-mailpit, etc.)
// This can be used by multiple projects simultaneously.
func StopLodevService() error {
	stopSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	stopSpin.Start("Stopping LODEV services")

	SetCommonEnv(nodeps.ServiceComposeProjectName)

	fullPath := LodevServices.ServiceComposeFullPath()

	if !fileutil.FileExists(fullPath) {
		err := fmt.Errorf("Compose file not found at expected path: %s", fullPath)
		stopSpin.Stop("Compose file not found", 2, tap.StopOptions{
			Hint: fmt.Sprintf("Compose file not found at expected path: %s", fullPath),
		})

		stopContainerSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
		stopContainerSpin.Start("Attempting to stop any running LODEV service containers")
		services, err := FindLodevServices()
		if err != nil {
			stopContainerSpin.Stop("No running LODEV service containers found", 0)
			return nil
		}

		for _, service := range services {
			err = dockerutil.RemoveContainer(service.ID)
			if err != nil {
				if ok := dockerutil.IsErrNotFound(err); !ok {
					stopContainerSpin.Stop(fmt.Sprintf("Failed to stop LODEV service %s: %v", service.Names[0][1:], err), 2)
					return err
				}
			}
		}

		stopContainerSpin.Stop("Stopped running LODEV service containers", 0)

		return nil
	}

	util.Debug("Executing docker-compose -f %s down", fullPath)
	_, err := ComposeCmd(&ComposeCmdOpts{
		ComposeFiles: []string{fullPath},
		Profiles:     []string{`*`},
		Action:       []string{"down"},
	})

	if err != nil {
		stopSpin.Stop(fmt.Sprintf("Failed to stop LODEV services: %v", err), 2)
		return fmt.Errorf("Failed to stop LODEV services: %v", err)
	}

	stopSpin.Stop("LODEV services stopped", 0)

	return nil
}

func FindLodevServices() ([]container.Summary, error) {
	query := map[string]string{
		dockerutil.LabelType:        dockerutil.LabelTypeService,
		"com.docker.compose.oneoff": "False",
	}
	c, err := dockerutil.FindContainersByLabels(query)
	if err != nil {
		return nil, fmt.Errorf("Failed to execute findContainersByLabels, %v", err)
	}
	if c == nil {
		return nil, fmt.Errorf("no %s was found", dockerutil.LabelTypeService)
	}
	return c, nil
}
