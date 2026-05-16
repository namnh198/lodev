package dockerutil

import (
	"maps"

	"github.com/namnh198/lodev/pkg/nodeps"
)

// Define Docker labels for LODEV-managed containers and resources.
const (
	LabelAppName  = "com.lodev.appname"
	LabelPlatform = "com.lodev.platform"
	LabelWebTag   = "com.lodev.webTag"
	LabelAppRoot  = "com.lodev.approot"
	LabelAppType  = "com.lodev.apptype"
	LabelType     = "com.lodev.type"
	LabelOneOff   = "com.docker.compose.oneoff"
	LabelService  = "com.docker.compose.service"

	LabelTypeService = "lodev-service"
	LabelTypeProject = "lodev-project"
	LabelTypeRouter  = "lodev-router"
)

// GetDockerLodevLabels returns a map of Docker labels for LODEV-managed containers and resources.
func GetDockerLodevLabels(appName string, labels map[string]string) map[string]string {
	defaultLabel := map[string]string{
		// Make sure to use same label for all containers those's managed by lodev
		LabelPlatform: "lodev",

		// Project name; services for shared services container; empty for global resources. Used by poweroff to remove
		// containers of a project when poweroff is run with --project flag. Also used by lodev ps to show project name in output.
		LabelAppName: appName,

		// Define web tag on resource. Currently the actively not used
		// but it determine resource that's stale or rebuilds when web tag is changed after LODEV update the new version.
		LabelWebTag: nodeps.DockerTag,
	}

	if labels == nil {
		return defaultLabel
	}

	maps.Copy(labels, defaultLabel)

	return labels
}

// GetDefaultDockerLodevLabels returns the default Docker labels for LODEV-managed containers and resources.
func GetDefaultDockerLodevLabels() map[string]string {
	return GetDockerLodevLabels("", nil)
}
