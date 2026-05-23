package nodeps

var DockerComposeVersion = ""

// constraints for docker images and containers used by LODEV
const (
	RouterContainer           = "lodev-router"
	RouterComposeProjectName  = "lodev-router"
	ServiceComposeProjectName = "lodev-service"
	RouterImage               = "namnh198/lodev-router"
	WebImage                  = "namnh198/lodev-webserver"
	UtilitiesImage            = "namnh198/lodev-utilities:latest"
	WebContainer              = "web"
	MinEphemeralPort          = 33000
	MaxEphemeralPort          = 35000
	LodevNetwork              = "lodev_default"
	LodevServiceNetwork       = "lodev_service"
	LodevWebWorkingDir        = "/var/www/html"
)

// Default configuration values for the LODEV application.
const (
	InternetWaitTimeout  = "300"
	ContainerWaitTimeout = "120"
	Router               = "traefik"
	HttpPort             = "80"
	HttpsPort            = "443"
	TraefikMonitorPort   = "10999"
	ProjectTld           = "test"
	RestartAlways        = true
)

const (
	// RequiredDockerComposeVerion defines the required version of docker-compose
	// Keep this in sync with github.com/compose-spec/compose-go/v2 in go.mod,
	// matching the version used in https://github.com/docker/compose/blob/main/go.mod for the same tag
	RequiredDockerComposeVerion = "v5.1.3"
	// RequiredDockerBuildxVersion defines the required version of docker buildx
	// LODEV used buildx features to build docker images. We recommend used the latest stable version (Currently v0.33.0)
	RequiredDockerBuildxVersion = "v0.33.0"

	// DockerTag is the tag used for LODEV's Docker images. It should be updated with each release.
	DockerTag = "v0.1.1"
)

// List of services defaultly connected to LODEV projects
var ConnectedServices = []string{"mysql", "phpmyadmin", "mailpit"}

// SharedAssets is the list of asset directories that are shared between all services and providers
var SharedAssets = []string{"commands", "homeadditions", "php", "web-build"}

// GetWebImage returns the full image reference for the web server
func GetWebImage() string {
	return WebImage + ":" + DockerTag
}

// GetRouterImage returns the full image reference for the router
func GetRouterImage() string {
	return RouterImage + ":" + DockerTag
}
