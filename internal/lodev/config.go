package lodev

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"go.yaml.in/yaml/v4"
)

// LodevConfigOpts is the struct defining LODEV's global config, it's stored in the ~/.lodev/global_config.yaml
type LodevConfigOpts struct {
	LastStartedVersion     string   `yaml:"last_started_version"`
	InternetWaitTimeout    string   `yaml:"internet_wait_timeout,omitempty"`
	ContainerWaitTimeout   string   `yaml:"container_wait_timeout,omitempty"`
	UseLetsEncrypt         bool     `yaml:"use_lets_encrypt"`
	LetsEncryptEmail       string   `yaml:"lets_encrypt_email"`
	Router                 string   `yaml:"router,omitempty"`
	ProjectTld             string   `yaml:"project_tld"`
	HttpPort               string   `yaml:"http_port"`
	HttpsPort              string   `yaml:"https_port"`
	TraefikMonitorPort     string   `yaml:"traefik_monitor_port"`
	WebEnvironment         []string `yaml:"web_environment"`
	ConnectedServices      []string `yaml:"connected_services,flow"`
	UseDockerComposeSystem bool     `yaml:"use_docker_compose_system,omitempty"`
	UseDockerBuildxSystem  bool     `yaml:"use_docker_buildx_system,omitempty"`
	MkcertCARoot           string   `yaml:"mkcert_ca_root,omitempty"`
	VersionConstraint      string   `yaml:"-"`
	SharedAssets           []string `yaml:"-"`
}

type ProjectRegistry struct {
	AppRoot   string   `yaml:"app_root"`
	UsedPorts []string `yaml:"used_ports,omitempty"`
}

var (
	LodevConfig           LodevConfigOpts             // LodevConfig is the global config for LODEV, read from ~/.lodev/global_config.yaml
	LodevProjectsRegistry map[string]*ProjectRegistry // LodevProjectsRegistry is the registry of all projects, it's stored in the ~/.lodev/projects_list.yaml
)

func EnsureLodevConfig() {
	LodevConfig = NewLodevConfig()
	LodevProjectsRegistry = make(map[string]*ProjectRegistry)

	util.Debug("Ensuring LODEV global config is loaded and valid")
	if err := ReadLodevConfig(); err != nil {
		util.Failed("Failed to read LODEV global config. Please recheck your config (%s): %v", GetLodevConfigPath("global_config.yaml"), err)
	}

	util.Debug("Ensuring LODEV projects registry is loaded and valid")
	if err := GetProjectsRegistry(); err != nil {
		util.Failed("Failed to read LODEV projects registry. Please recheck your config (%s): %v", GetLodevConfigPath("project_list.yaml"), err)
	}
}

// NewLodevConfig returns a new instance of LodevConfigOpts with default values
func NewLodevConfig() LodevConfigOpts {
	cfg := LodevConfigOpts{
		LastStartedVersion:   "v0.0", // override with the actual version when starting LODEV
		InternetWaitTimeout:  nodeps.InternetWaitTimeout,
		ContainerWaitTimeout: nodeps.ContainerWaitTimeout,
		Router:               nodeps.Router,
		HttpPort:             nodeps.HttpPort,
		HttpsPort:            nodeps.HttpsPort,
		TraefikMonitorPort:   nodeps.TraefikMonitorPort,
		ProjectTld:           nodeps.ProjectTld,
		VersionConstraint:    nodeps.VersionConstraint,
		MkcertCARoot:         readCAROOT(),
		SharedAssets:         nodeps.SharedAssets,
	}

	return cfg
}

// ReadLODEVConfig reads the LODEV global config from the config file (~/.lodev/global_config.yaml) into LodevConfig
// Or create the file with default config if it doesn't exist
func ReadLodevConfig() (err error) {
	configFile := GetLodevConfigPath("global_config.yaml")
	if _, err = os.Stat(configFile); err != nil {
		// Create a new config file with default values if it doesn't exist, but don't create it if running with root privileges
		if os.Geteuid() == 0 {
			util.Warning("Not reading configuration file because running with root privileges.")
			return err
		}

		// Create a config file with default values, return the error with other cases
		if os.IsNotExist(err) {
			util.Debug("Config File %s does not exist. Trying to create it.", GetLodevConfigDir())
			LodevConfig = NewLodevConfig()
			if err = SaveLodevConfig(); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if err = util.LoadYAML(configFile, &LodevConfig); err != nil {
		return err
	}

	caRootEnv := os.Getenv("CAROOT")
	if GetCAROOT() == "" || !fileutil.FileExists(filepath.Join(LodevConfig.MkcertCARoot, "rootCA.pem")) || (caRootEnv != "" && caRootEnv != LodevConfig.MkcertCARoot) {
		LodevConfig.MkcertCARoot = readCAROOT()
	}

	if LodevConfig.InternetWaitTimeout < nodeps.InternetWaitTimeout {
		LodevConfig.InternetWaitTimeout = nodeps.InternetWaitTimeout
	}

	if LodevConfig.ContainerWaitTimeout < nodeps.ContainerWaitTimeout {
		LodevConfig.ContainerWaitTimeout = nodeps.ContainerWaitTimeout
	}

	if LodevConfig.HttpPort == "" {
		LodevConfig.HttpPort = nodeps.HttpPort
	}

	if LodevConfig.HttpsPort == "" {
		LodevConfig.HttpsPort = nodeps.HttpsPort
	}

	if LodevConfig.TraefikMonitorPort == "" {
		LodevConfig.TraefikMonitorPort = nodeps.TraefikMonitorPort
	}

	if LodevConfig.ProjectTld == "" {
		LodevConfig.ProjectTld = nodeps.ProjectTld
	}

	LodevConfig.ConnectedServices = util.SliceToUniqueSlice(&LodevConfig.ConnectedServices)
	LodevConfig.WebEnvironment = util.EnvToUniqueEnv(&LodevConfig.WebEnvironment)

	err = ValidateLodevConfig()

	return nil
}

// readCAROOT() verifies that the mkcert command is available and its CA keys readable.
// 1. Find out CAROOT
// 2. Look there to see if key/crt are readable
// 3. If not, see if mkcert is even available, return empty
func readCAROOT() string {
	_, err := exec.LookPath("mkcert")
	if err != nil {
		return ""
	}

	out, err := exec.Command("mkcert", "-CAROOT").Output()
	if err != nil {
		return ""
	}
	root := strings.Trim(string(out), "\r\n")
	rootPem := filepath.Join(root, "rootCA.pem")

	if !fileutil.IsReadable(rootPem) || !fileutil.FileExists(rootPem) {
		return ""
	}

	return root
}

// GetCAROOT is a wrapper on global config
func GetCAROOT() string {
	_, err := exec.LookPath("mkcert")
	if err != nil {
		return ""
	}
	return LodevConfig.MkcertCARoot
}

// GetLODEVConfigPath returns the LODEV global config path and creates it if as needed
func GetLodevConfigPath(path ...string) string {
	lodevDir := GetLodevConfigDir()

	// create the ~/.lodev directory if it doesn't exist, but don't create it if running with root privileges
	if !fileutil.FileExists(lodevDir) {
		if os.Getuid() == 0 {
			util.Warning("Not creating the %s since you're running with root privileges", lodevDir)
		} else if err := os.MkdirAll(lodevDir, 0755); err != nil {
			util.Failed("Failed to create required directory %s, ERR: %v", lodevDir, err)
		}
	}

	return filepath.Join(lodevDir, filepath.Join(path...))
}

// SaveLodevConfig writes the LODEV global config from LodevConfig to the config file (~/.lodev/global_config.yaml)
func SaveLodevConfig() (err error) {
	if err = ValidateLodevConfig(); err != nil {
		return
	}

	cfgCopy := LodevConfig

	cfgbytes, err := yaml.Marshal(cfgCopy)
	cfgbytes = append(cfgbytes, []byte(nodeps.InstructmentationConfig)...)

	if err != nil {
		return
	}

	configFile := GetLodevConfigPath("global_config.yaml")
	os.WriteFile(configFile, cfgbytes, 0644)

	return
}

// ValidateLodevConfig validates the LODEV global config
func ValidateLodevConfig() error {
	// Force router to traefik for now, as it's the only supported router, and we want to avoid some unexpected issues
	if LodevConfig.Router != nodeps.Router {
		util.Warning(
			"\nCurrently, LODEV only support %s router, but you have router: %s in your configuration, using %s instead\n",
			nodeps.Router,
			LodevConfig.Router,
			nodeps.Router,
		)
		LodevConfig.Router = nodeps.Router
	}

	// if len(LodevConfig.ConnectedServices) == 0 {
	// 	return fmt.Errorf("The connected services can not be empty.\nIt should be get a database service like 'mysql' or 'mariadb'.")
	// }

	// Force VersionConstraint to default value that's we expected, avoid some issues
	LodevConfig.VersionConstraint = nodeps.VersionConstraint

	// Force SharedAssets to default value that's we expected, avoid some issues
	LodevConfig.SharedAssets = nodeps.SharedAssets

	return nil
}

// GetProjectsRegistry read the global projects registry into LodevProjectsRegistry
// Or create an empty project list if the file ~/.lodev/project_list.yaml does not exist
func GetProjectsRegistry() (err error) {
	projectFile := GetLodevConfigPath("project_list.yaml")

	if !fileutil.FileExists(projectFile) {
		if os.Getuid() == 0 {
			util.Warning("Not creating the %s since you're running with root privileges", projectFile)
		} else if err = SaveProjectsRegistry(); err != nil {
			return
		}
	}

	err = util.LoadYAML(projectFile, &LodevProjectsRegistry)

	return
}

// SaveProjectsRegistry writes the global projects state into the ~/.lodev/project_list.yaml file
func SaveProjectsRegistry() (err error) {
	projectFile := GetLodevConfigPath("project_list.yaml")
	projectBytes, err := yaml.Marshal(LodevProjectsRegistry)
	if err != nil {
		return
	}

	err = os.WriteFile(projectFile, projectBytes, 0644)

	return
}

// GetLodevBinPath returns the bin directory that states 3th-party binary (mutagen, docker-compose)
func GetLodevBinPath(path ...string) string {
	lodevBinDir := GetLodevConfigPath("bin")
	return filepath.Join(lodevBinDir, filepath.Join(path...))
}

// GetLodevAddonsPath returns the addons directory that states 3th-party addons
func GetLodevServicePath(path ...string) string {
	lodevAddonsDir := GetLodevConfigPath("services")
	return filepath.Join(lodevAddonsDir, filepath.Join(path...))
}

// GetLodevConfigDir returns the global caching directory location to be used by LODEV:
// $XDG_CONFIG_HOME/lodev if this $XDG_CONFIG_HOME variable is not empty,
// ~/.config/lodev if this directory exists on Linux/WSL2 only,
// ~/.lodev otherwise
func GetLodevConfigDir() string {
	userHome, err := os.UserHomeDir()
	if err != nil {
		util.Failed("Could not get home directory for current user. ERR: %v", err)
	}

	lodevPath := os.Getenv("LODEV_CONFIG_DIR")
	if strings.HasPrefix(lodevPath, "~") {
		lodevPath = userHome + lodevPath[1:]
	}
	if lodevPath != "" {
		os.Setenv("LODEV_CONFIG_DIR", lodevPath)
		return lodevPath
	}

	userHomeDot := filepath.Join(userHome, ".lodev")

	xdgConfigHomeDir := os.Getenv("XDG_CONFIG_HOME")
	// Handle ~/xxx without failure; MUTAGEN_DATA_DIRECTORY, for example, can't have it.
	if strings.HasPrefix(xdgConfigHomeDir, `~`) {
		xdgConfigHomeDir = userHome + xdgConfigHomeDir[1:]
	}

	if xdgConfigHomeDir != "" {
		lodevPath = filepath.Join(xdgConfigHomeDir, "lodev")
		os.Setenv("LODEV_CONFIG_DIR", lodevPath)
		return lodevPath
	}

	// If Linux and ~/.lodev doesn't exist and
	// ~/.config/lodev exists, use it,
	// we don't create this directory.
	stat, userHomeDotErr := os.Stat(userHomeDot)
	userHomeDotIsDir := userHomeDotErr == nil && stat.IsDir()
	if util.IsLinux() && !userHomeDotIsDir {
		userConfigDir, err := os.UserConfigDir()
		if err == nil {
			linuxDir := filepath.Join(userConfigDir, "lodev")
			if _, err := os.Stat(linuxDir); err == nil {
				os.Setenv("LODEV_CONFIG_DIR", linuxDir)
				return linuxDir
			}
		}
	}
	// Otherwise, use ~/.lodev
	// It will be created if it doesn't exist.
	os.Setenv("LODEV_CONFIG_DIR", userHomeDot)
	return userHomeDot
}
