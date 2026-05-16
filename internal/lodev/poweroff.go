package lodev

import (
	"fmt"

	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/yarlson/tap"
)

func PowerOff() {
	var err error

	projects := GetActiveProjects()

	for _, project := range projects {
		if err = project.Stop(false); err != nil {
			util.WarningMessage(fmt.Sprintf("Failed to stop project %s: %v", project.GetName(), err))
		}
	}

	if err = StopLodevService(); err != nil {
		util.WarningMessage(fmt.Sprintf("Failed to stop services: %v", err))
	}

	if err := StopLodevRouter(); err != nil {
		util.WarningMessage(fmt.Sprintf("Failed to stop %s: %v", nodeps.RouterContainer, err))
	}

	containers, err := dockerutil.FindContainersByLabels(map[string]string{dockerutil.LabelAppName: ""})

	if err == nil {
		for _, c := range containers {
			err = dockerutil.RemoveContainer(c.ID)
			if err != nil {
				util.WarningMessage(fmt.Sprintf("Failed to remove container %+v", c))
			}
		}
	} else {
		util.WarningMessage(fmt.Sprintf("Unable to run client.ListContainers(): %v", err))
	}
	networkSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	networkSpin.Start("Removing LODEV networks")
	// Remove all networks created with LODEV
	removals, err := dockerutil.FindNetworksWithLabel(dockerutil.LabelPlatform)
	if err == nil {
		for _, network := range removals {
			err := dockerutil.RemoveNetwork(network.Name)
			if err != nil && !dockerutil.IsErrNotFound(err) {
				networkSpin.Message(fmt.Sprintf("Failed to remove network %s: %v", network.Name, err))
			} else if err == nil {
				networkSpin.Message(fmt.Sprintf("Network %s removed", network.Name))
			}
		}
	} else {
		networkSpin.Stop(fmt.Sprintf("Unable to run dockerutil.FindNetworksWithLabel(): %v", err), 2)
		return
	}

	networkSpin.Stop("LODEV networks removed", 0)
}
