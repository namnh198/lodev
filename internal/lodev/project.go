package lodev

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/yarlson/tap"
	"go.yaml.in/yaml/v4"
)

var RunValidate = true // Flag to control whether to run project validation (No need to run validation if removing a project)

const LodevDir = ".lodev" // Constant for the LODEV directory name

// NewProject creates a new Project struct with the given project root and default values.
func NewProject(appRoot string) (*Project, error) {
	project := &Project{}

	if appRoot == "" {
		appRoot, _ = os.Getwd()
	}
	project.AppRoot = appRoot

	if err := CanCreateProject(project.AppRoot); err != nil {
		return project, err
	}

	if _, err := os.Stat(project.AppRoot); err != nil {
		return project, err
	}
	project.Name = NormalizeProjectName(filepath.Base(project.AppRoot))
	project.ConfigFile = project.GetConfigFile()
	project.DetectProjectType()
	project.PHPVersion = nodeps.DefaultPHPVersion
	project.NodeJSVersion = nodeps.DefaultNodeJSVersion
	project.ComposerVersion = nodeps.DefaultComposerVersion
	project.Webserver = nodeps.DefaultWebserver
	project.RestartAlways = nodeps.RestartAlways
	project.WebImage = nodeps.GetWebImage()
	project.WorkingDir = nodeps.LodevWebWorkingDir

	err := project.ReadProjectConfig()
	if err != nil {
		return project, fmt.Errorf("%v exist but could not read. Maybe invalid due to syntax error: %v", project.ConfigFile, err)
	}

	return project, nil
}

// InitProject initializes the project by validating the config and checking for existing containers with the same project name.
func (project *Project) InitProject(basePath string) error {
	newProject, err := NewProject(basePath)
	if err != nil {
		return err
	}

	err = newProject.ValidateProjectConfig()
	if err != nil {
		return err
	}

	*project = *newProject

	web, err := FindContainerByType(nodeps.WebContainer, project.Name)
	if err != nil {
		return err
	}

	if web != nil {
		containerApproot := web.Labels[dockerutil.LabelAppRoot]
		isSameFile, err := fileutil.IsSameFile(containerApproot, project.AppRoot)
		if err != nil {
			return err
		}
		if !isSameFile {
			return fmt.Errorf("a project (web container) in %s state already exists for %s that was created at %s", web.State, project.Name, containerApproot)
		}
	}

	// Init() is putting together the Project struct, the containers do
	// not have to exist (app doesn't have to have been started), so the fact
	// we didn't find any is not an error.
	return nil
}

// GetProjects returns projects that are listed
// in globalconfig projectlist (or in Docker container labels, or both)
// if activeOnly is true, only show projects that aren't stopped
// (or broken, missing config, missing files)
func GetProjects(activeOnly bool) ([]*Project, error) {
	apps := make(map[string]*Project)
	// First grab the GetActiveApps (Docker labels) version of the projects and make sure it's
	// included. Hopefully Docker label information and global config information will not
	// be out of sync very often.
	activeProjects := GetActiveProjects()
	for _, app := range activeProjects {
		apps[app.Name] = app
	}

	// Now get everything we can find in global project list
	for name, info := range LodevProjectsRegistry {
		// Skip apps already found running in Docker
		if _, ok := apps[name]; ok {
			continue
		}

		project, err := NewProject(info.AppRoot)
		if err != nil {
			if os.IsNotExist(err) {
				util.Warning("The project '%s' no longer exists in the filesystem, removing it from registry", info.AppRoot)
				err = RemoveProjectRegistry(name)
				if err != nil {
					util.Warning("unable to RemoveProjectRegistry(%s): %v", name, err)
				}
			} else {
				util.Warning("Something went wrong with %s: %v", info.AppRoot, err)
			}
			continue
		}

		// If the app we loaded was already found with a different name, complain
		if _, ok := apps[project.Name]; ok {
			util.Warning(`Project '%s' was found in configured directory %s and it is already used by project '%s'. If you have changed the name of the project, please "lodev stop --unlist %s" `, project.Name, project.AppRoot, name, name)
			continue
		}

		status := project.SiteStatus()
		if !activeOnly || (status != SiteStopped && status != SiteConfigMissing && status != SiteDirMissing) {
			apps[project.Name] = project
		}
	}

	projectSlice := []*Project{}
	for _, v := range apps {
		projectSlice = append(projectSlice, v)
	}
	sort.Slice(projectSlice, func(i, j int) bool { return projectSlice[i].Name < projectSlice[j].Name })

	return projectSlice, nil
}

// GetActiveProjects returns an array of LODEV projects
// that are currently running in docker (excludes paused/stopped projects).
func GetActiveProjects() []*Project {
	projects := make([]*Project, 0)
	labels := map[string]string{
		dockerutil.LabelPlatform:     "lodev",
		"com.docker.compose.service": "web",
		"com.docker.compose.oneoff":  "False",
	}
	containers, err := dockerutil.FindContainersByLabels(labels)

	if err == nil {
		for _, siteContainer := range containers {
			// Skip containers that are not running (e.g., paused projects)
			if siteContainer.State != "running" {
				continue
			}
			approot, ok := siteContainer.Labels[dockerutil.LabelAppRoot]
			if !ok {
				continue
			}

			app, err := NewProject(approot)
			// Artificially populate sitename and apptype based on labels
			// if NewApp() failed.
			if err != nil {
				app.Name = siteContainer.Labels[dockerutil.LabelAppName]
				app.Type = siteContainer.Labels[dockerutil.LabelAppType]
				app.AppRoot = siteContainer.Labels[dockerutil.LabelAppRoot]
			}
			projects = append(projects, app)
		}
	}

	return projects
}

// GetActiveProject returns the active app based on the current working directory or an error if no active app is found
func GetActiveProject(name string) (*Project, error) {
	project := &Project{}
	activeRoot, err := GetProjectAppRootByName(name)
	if err != nil {
		return project, err
	}

	if err := project.InitProject(activeRoot); err != nil {
		return project, err
	}

	if project.Name == "" && name != "" {
		project.Name = name
	}

	return project, nil
}

// GetProjectAppRootByName returns the fully rooted directory of the active app, or an error
func GetProjectAppRootByName(prName string) (basePath string, err error) {
	if prName == "" {
		basePath, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current working directory: %v", err)
		}
		if _, err = IsProjectExists(basePath); err != nil {
			return "", fmt.Errorf("could not find a project in %s. Please specify a project name or change directories: %v", basePath, err)
		}
	} else if p, ok := LodevProjectsRegistry[prName]; ok {
		return p.AppRoot, nil
	} else {
		var ok bool
		webContainer, err := FindContainerByType(nodeps.WebContainer, prName)
		if err != nil {
			return "", fmt.Errorf("could not find a project with name %s: %v", prName, err)
		}
		if !ok {
			return "", fmt.Errorf("Project not found: %s", prName)
		}
		if basePath, ok = webContainer.Labels[dockerutil.LabelAppRoot]; !ok {
			return "", fmt.Errorf("could not determine the app root for project %s from container: %s", prName, dockerutil.ContainerName(webContainer))
		}
	}

	if basePath, err = IsProjectExists(basePath); err != nil {
		return "", err
	}

	return basePath, nil
}

// ValidateProjectConfig validates the project config and returns an error if any of the validation fails.
func (p *Project) ValidateProjectConfig() (err error) {
	if !RunValidate {
		return
	}
	// Validate project name
	if err = ValidateProjectName(p.Name); err != nil {
		return
	}

	// Validate project type
	if err = ValidateProjectType(p.Type); err != nil {
		return
	}

	// Validate docroot
	if err := ValidateDocroot(p.Docroot); err != nil {
		return err
	}

	// Validate PHP version
	if err = ValidatePHPVersion(p.PHPVersion); err != nil {
		return
	}

	// Validate webserver
	if err = ValidateWebserver(p.Webserver); err != nil {
		return
	}

	return
}

// ReadProjectConfig reads the project config from the config file, if it exists.
// If it doesn't exist, it creates a new config file with default values.
func (p *Project) ReadProjectConfig() error {
	if p.ConfigFile == "" {
		p.ConfigFile = p.GetConfigFile()
	}

	if p.NodeJSVersion == "" {
		p.NodeJSVersion = nodeps.DefaultNodeJSVersion
	}

	if !fileutil.FileExists(p.ConfigFile) {
		if err := p.WriteProjectConfig(); err != nil {
			return err
		}

		return nil
	}

	p.SortWebExtraExposedPorts()

	return util.LoadYAML(p.ConfigFile, p)
}

// WriteProjectConfig writes the app config to the config file
func (p *Project) WriteProjectConfig() error {
	// Work against a copy the Project, since we don't want to actually modify
	pCopy := *p

	if pCopy.WebImage == nodeps.GetWebImage() {
		pCopy.WebImage = ""
	}

	// Ensure valid app type
	if pCopy.Type == "" {
		p.DetectProjectType()
	}

	if IsComposerV1(pCopy.ComposerVersion) {
		pCopy.ComposerVersion = "2.2"
		util.Warning(`Project %s nows use Composer v2.2 LTS. Composer v1 is no longer support, see https://blog.packagist.com/shutting-down-packagist-org-support-for-composer-1-x/`, p.Name)
	}

	err := PrepLodevDirectory(&pCopy)
	if err != nil {
		return err
	}

	cfgbytes, err := yaml.Marshal(pCopy)
	if err != nil {
		return err
	}

	cfgbytes = append(cfgbytes, []byte(nodeps.InstructmentationProject)...)

	portsToReserve := []string{}
	if p.HostWebserverPort != "" {
		portsToReserve = append(portsToReserve, p.HostWebserverPort)
	}
	if p.HostHttpsPort != "" {
		portsToReserve = append(portsToReserve, p.HostHttpsPort)
	}
	for _, port := range p.WebExtraExposedPorts {
		portsToReserve = append(portsToReserve, strconv.Itoa(port.ContainerPort))
	}

	if len(portsToReserve) > 0 {
		if err = CheckHostPortsAvailable(p.Name, portsToReserve); err != nil {
			return err
		}
	}

	// We now want to reserve the port we're writing for HostDBPort and HostWebserverPort and so they don't
	// accidentally get used for other projects.
	err = AddProjectRegistry(p.Name, p.AppRoot, portsToReserve...)
	if err != nil {
		return err
	}

	err = os.WriteFile(pCopy.ConfigFile, cfgbytes, 0o644)
	if err != nil {
		return err
	}

	return nil
}

// HostPostIsAllocated returns the project name that has allocated
// the port, or empty string.
func HostPostIsAllocated(port string) string {
	for project, item := range LodevProjectsRegistry {
		if slices.Contains(item.UsedPorts, port) {
			return project
		}
	}
	return ""
}

// CheckHostPortsAvailable checks LodevProjectsRegistry UsedHostPorts to see if requested ports are available.
func CheckHostPortsAvailable(projectName string, ports []string) error {
	for _, port := range ports {
		allocatedProject := HostPostIsAllocated(port)
		if allocatedProject != projectName && allocatedProject != "" {
			return fmt.Errorf("host port %s has already been allocated to project %s", port, allocatedProject)
		}
	}
	return nil
}

// GetFreePort gets an ephemeral port currently available, but also not
// listed in LodevProjectsRegistry.UsedHostPorts
func GetFreePort(localIPAddr string) (string, error) {
	// Limit tries arbitrarily. It will normally succeed on first try.
	for i := 1; i < 1000; i++ {
		// From https://github.com/phayes/freeport/blob/master/freeport.go#L8
		// Ignores that the actual listener may be on a Docker toolbox interface,
		// so this is a heuristic.
		addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return "", err
		}
		port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
		// nolint: errcheck
		l.Close()

		// In the case of Docker Toolbox, the actual listening IP may be something else
		// like 192.168.99.100, so check that to make sure it's not currently occupied.
		conn, _ := net.Dial("tcp", localIPAddr+":"+port)
		if conn != nil {
			continue
		}

		if HostPostIsAllocated(port) != "" {
			continue
		}
		return port, nil
	}
	return "-1", fmt.Errorf("getFreePort() failed to find a free port")
}

// PromptCreateProject prompts the user to enter the necessary information to create a new LODEV p.
func (p *Project) PromptCreateProject(skipPromptArg map[string]bool) {
	tap.Intro("\n🚀 Create a LODEV project", tap.MessageOptions{
		Hint: fmt.Sprintf("AppRoot: %s", p.AppRoot),
	})

	// project name prompt
	if ok := skipPromptArg["project-name"]; !ok {
		p.Name = tap.Text(context.Background(), tap.TextOptions{
			Message:      fmt.Sprintf("What's your project named? (default: %s): ", p.GetName()),
			Placeholder:  p.Name,
			Validate:     ValidateProjectName,
			DefaultValue: p.Name,
		})
	}

	// Ask if user want to use recommended default values for the rest of the prompts, if not, ask for each of them.
	useRecommended := tap.Confirm(context.Background(), tap.ConfirmOptions{
		Message:      "Use recommended default values for the rest of the prompts? (default: yes): ",
		InitialValue: true,
	})

	if useRecommended {
		return
	}

	// project type prompt
	if ok := skipPromptArg["project-type"]; !ok {
		p.Type = tap.Autocomplete(context.Background(), tap.AutocompleteOptions{
			Message:     fmt.Sprintf("Project Type (default: %s):", p.Type),
			Placeholder: "Start typing...",
			Suggest:     sugggestPrompt(GetProjectTypes()),
			MaxResults:  6,
			Validate:    ValidateProjectType,
		})
	}

	// docroot prompt
	if ok := skipPromptArg["docroot"]; !ok {
		defaultDocroot := p.GetDocroot()
		docrootPrompt := "Document Root (default: %s): "
		if p.Docroot != "" {
			docrootPrompt = fmt.Sprintf(docrootPrompt, p.Docroot)
		} else {
			docrootPrompt = fmt.Sprintf(docrootPrompt, "project root")
		}
		p.Docroot = tap.Autocomplete(context.Background(), tap.AutocompleteOptions{
			Message:      docrootPrompt,
			Placeholder:  "Start typing...",
			Suggest:      sugggestPrompt(AvailablePHPDocrootLocations()),
			MaxResults:   6,
			InitialValue: defaultDocroot,
			Validate:     ValidateDocroot,
		})
	}

	// php version prompt
	if ok := skipPromptArg["php-version"]; !ok {
		p.PHPVersion = tap.Select(context.Background(), tap.SelectOptions[string]{
			Message:      fmt.Sprintf("PHP Version (default: %s):", nodeps.DefaultPHPVersion),
			InitialValue: &p.PHPVersion,
			Options:      getSelectOptionsPrompt(nodeps.ValidPHPVersions),
		})
	}

	// webserver prompt
	if ok := skipPromptArg["webserver-version"]; !ok {
		p.Webserver = tap.Select(context.Background(), tap.SelectOptions[string]{
			Message:      fmt.Sprintf("Webserver (default: %s):", nodeps.DefaultWebserver),
			InitialValue: &p.Webserver,
			Options:      getSelectOptionsPrompt(nodeps.ValidWebservers),
		})
	}
}

// WarnIfConfigReplace messages user about whether config is being replaced or created
func WarnIfProjectExists(appRoot string) {
	if _, err := IsProjectExists(appRoot); err == nil {
		util.WarningMessage(fmt.Sprintf("You are reconfiguring the project at %s", appRoot), tap.MessageOptions{
			Hint: "The existing configuration will be updated and replaced.",
		})
	} else {
		configFile := filepath.Join(appRoot, LodevDir, "config.yaml")
		util.SuccessMessage(fmt.Sprintf("Creating a new LODEV project config in the current directory (%s)", appRoot), tap.MessageOptions{
			Hint: fmt.Sprintf("Once completed, your configuration will be written to %s\n", configFile),
		})
	}
}

// CanCreateProject checks if the project can be created in the given project root directory.
//
// - Not the home directory,not a parent of the home directory,
// - Not the LODEV source code directory.
// - No existing projects in the parent directories.
func CanCreateProject(appRoot string) error {
	// Do not run this check if we want to delete the p.
	if !RunValidate {
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("Failed to get user home directory: %v", err)
	}

	if appRoot == homeDir || appRoot == filepath.Dir(GetLodevConfigDir()) {
		return fmt.Errorf("You cannot create a LODEV project in your home directory. Please choose another directory.")
	}

	rel, err := filepath.Rel(appRoot, homeDir)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return fmt.Errorf("A project is not allowed in the parent directory of your home directory . ERR: %v", err)
	}

	if fileutil.FileExists(filepath.Join(appRoot, "internal", "lodev", "lodev.go")) {
		return fmt.Errorf("A project cannot be created in the LODEV source code (%v)", appRoot)
	}

	// If this is an existing project, allow it
	if _, err := IsProjectExists(appRoot); err == nil {
		return nil
	}

	prStates := make([]*ProjectRegistry, 0, len(LodevProjectsRegistry))
	for _, state := range LodevProjectsRegistry {
		prStates = append(prStates, state)
	}

	// Sort the project by AppRoot in reserve alphabetical order, this ensures that sub-directory project are checked first
	sort.Slice(prStates, func(i, j int) bool {
		return prStates[i].AppRoot > prStates[j].AppRoot
	})

	for _, prState := range prStates {
		// Without sorting, a parent directory might be matched first, causing the function to return without checking the project in the subdirectory
		if appRoot == prState.AppRoot {
			return nil
		}
		// Do not allow 'lodev init' in the parent directory of an existing project
		rel, err := filepath.Rel(appRoot, prState.AppRoot)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return fmt.Errorf("A project is not allowed in (%s) because another existing in subdirectory %s\nUnlist this project (if it exists) with 'cd %s && lodev stop --unlist' for all project in thge subdirectories of this project directory", appRoot, prState.AppRoot, prState.AppRoot)
		}
	}

	return nil
}

// PrepLodevDirectory creates a .lodev directory in the current working directory
func PrepLodevDirectory(p *Project) error {
	dir := p.GetConfigPath()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0o755)
		if err != nil {
			return err
		}
	}

	// err = PopulateLodevAssetsAndCommands(pCopy.Name)
	// if err != nil {
	// 	return err
	// }

	return nil
}

// IsProjectExists check if a LODEV project exists in the given directory
func IsProjectExists(appRoot string) (string, error) {
	configPath := filepath.Join(appRoot, LodevDir, "config.yaml")
	if fileutil.FileExists(configPath) {
		return appRoot, nil
	}
	// Keep going until we can't go any higher
	for filepath.Dir(appRoot) != appRoot {
		appRoot = filepath.Dir(appRoot)
		if fileutil.FileExists(filepath.Join(appRoot, LodevDir, "config.yaml")) {
			return appRoot, nil
		}
	}

	return "", fmt.Errorf("no %s file was found in this directory or any parent", filepath.Join(LodevDir, "config.yaml"))
}

// GetConfigPath returns the absolute path to the given path relative to the project root directory
func (p *Project) GetConfigPath(path ...string) string {
	return filepath.Join(p.AppRoot, LodevDir, filepath.Join(path...))
}

// GetConfigFile returns the absolute path to the project's config.yaml file
func (p *Project) GetConfigFile() string {
	return p.GetConfigPath("config.yaml")
}

// RemoveProjectRegistry removes the project registry with the given project name from the global project registry
func RemoveProjectRegistry(projectName string) error {
	if _, ok := LodevProjectsRegistry[projectName]; ok {
		delete(LodevProjectsRegistry, projectName)
		err := SaveProjectsRegistry()
		if err != nil {
			return fmt.Errorf("Failed to write project list: %v", err)
		}
	}
	return nil
}

// AddProjectRegistry adds a project registry to the global project registry list and writes it to the ~/.lodev/project_list.yaml file
func AddProjectRegistry(projectName string, appRoot string, usedPorts ...string) error {
	if projectName == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	if _, ok := LodevProjectsRegistry[projectName]; !ok {
		LodevProjectsRegistry[projectName] = &ProjectRegistry{}
	}

	if _, err := os.Stat(appRoot); err != nil {
		return fmt.Errorf("Project '%s' project root %s does not exists.", projectName, appRoot)
	}

	if LodevProjectsRegistry[projectName].AppRoot != "" && LodevProjectsRegistry[projectName].AppRoot != appRoot {
		return fmt.Errorf(
			"Project '%s' project root is already set to %s, refusing to change it to %s; you can `lodev stop --unlist %s` and start again.",
			projectName,
			LodevProjectsRegistry[projectName].AppRoot,
			appRoot,
			projectName,
		)
	}

	LodevProjectsRegistry[projectName].AppRoot = appRoot
	LodevProjectsRegistry[projectName].UsedPorts = usedPorts
	err := SaveProjectsRegistry()
	if err != nil {
		return fmt.Errorf("Failed to write project list: %v", err)
	}

	return nil
}
