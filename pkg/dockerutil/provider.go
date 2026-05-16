package dockerutil

import (
	"slices"
	"strings"

	"github.com/namnh198/lodev/pkg/util"
)

// IsDockerDesktop detects if running on Docker Desktop
func IsDockerDesktop() bool {
	info, err := GetDockerClientInfo()
	if err != nil {
		return false
	}
	if strings.HasPrefix(info.OperatingSystem, "Docker Desktop") {
		return true
	}
	if strings.Contains(info.Name, "docker-desktop") {
		return true
	}
	return false
}

// IsOrbStack detects if running on OrbStack
func IsOrbStack() bool {
	info, err := GetDockerClientInfo()
	if err != nil {
		return false
	}
	if strings.HasPrefix(info.OperatingSystem, "OrbStack") {
		return true
	}
	if strings.Contains(info.Name, "orbstack") {
		return true
	}
	return false
}

// IsRootless detects if Docker is running in rootless mode
func IsRootless() bool {
	info, err := GetDockerClientInfo()
	if err != nil {
		return false
	}
	return slices.Contains(info.SecurityOptions, "name=rootless")
}

// IsDockerRootless detects if Docker is running in rootless mode on Linux
// It must not be Podman or Lima, which can be rootless as well.
func IsDockerRootless() bool {
	return IsRootless() && util.IsLinux()
}
