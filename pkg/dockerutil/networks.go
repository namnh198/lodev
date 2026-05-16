package dockerutil

import (
	"fmt"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/namnh198/lodev/pkg/util"
)

// EnsureNetwork will ensure the Docker network for LODEV is created.
func EnsureNetwork(netName string, netOptions client.NetworkCreateOptions) error {
	RemoveNetworkDuplicate(netName)

	if !NetWorkExists(netName) {
		if err := CreateNetwork(netName, netOptions); err != nil {
			return err
		}
		util.InfoMessage(fmt.Sprintf("Network (%s) created", netName))
	}

	return nil
}

// CreateNetwork creates a Docker network with the given name and options
func CreateNetwork(netName string, netnetOptions client.NetworkCreateOptions) error {
	ctx, apiClient, err := GetDockerClient()

	if err != nil {
		return err
	}

	_, err = apiClient.NetworkCreate(ctx, netName, netnetOptions)

	return err
}

// NetExists checks to see if the Docker network existing
// netName can also be network's Name or network's ID
func NetWorkExists(netName string) bool {
	netName = strings.ToLower(netName)
	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return false
	}

	nets, err := apiClient.NetworkList(ctx, client.NetworkListOptions{})

	if err != nil {
		return false
	}

	for _, net := range nets.Items {
		if net.Name == netName || net.ID == netName {
			return true
		}
	}

	return false
}

// FindNetworksWithLabel returns all networks with the given label
// It ignores the value of the label, is only interested that the label exists.
func FindNetworksWithLabel(label string) ([]network.Summary, error) {
	ctx, apiClient, err := GetDockerClient()

	if err != nil {
		return nil, err
	}

	nets, err := apiClient.NetworkList(ctx, client.NetworkListOptions{})

	if err != nil {
		return nil, err
	}

	var matchingNetworks []network.Summary

	for _, net := range nets.Items {
		if net.Labels == nil {
			continue
		}

		if _, ok := net.Labels[label]; ok {
			matchingNetworks = append(matchingNetworks, net)
		}
	}

	return matchingNetworks, nil
}

// RemoveNetworkWithWarningOnError removes the named Docker network and utils a warning if has any errors
func RemoveNetworkWithWarn(netName string) {
	err := RemoveNetwork(netName)

	if err != nil && errdefs.IsNotFound(err) {
		util.WarningMessage(fmt.Sprintf("Unable to remove network (%s). Err: %v", netName, err))
	} else if err != nil {
		util.InfoMessage(fmt.Sprintf("Network (%s) removed.", netName))
	}
}

// RemoveNetwork removes the named Docker network
// netName can also be network's Name or network's ID
func RemoveNetwork(netName string) error {
	ctx, apiClient, err := GetDockerClient()

	if err != nil {
		return err
	}

	nets, err := apiClient.NetworkList(ctx, client.NetworkListOptions{})

	if err != nil {
		return err
	}

	err = errdefs.ErrNotFound

	for _, net := range nets.Items {
		// Need to loop networks because maybe have multiple networks with same label
		// and delete only by ID - it's unique but netName isn't
		if net.Name == netName || net.ID == netName {
			_, err = apiClient.NetworkRemove(ctx, net.ID, client.NetworkRemoveOptions{})
		}
	}

	return err
}

// RemoveNetworkDuplicate removes duplicate network with same netName or ID, keeps one of them
// Ensure that is only one network with the given netName or ID exists
func RemoveNetworkDuplicate(netName string) {
	ctx, apiClient, err := GetDockerClient()

	if err != nil {
		return
	}

	nets, err := apiClient.NetworkList(ctx, client.NetworkListOptions{})

	if err != nil {
		return
	}

	networkMatchFound := false

	for _, net := range nets.Items {
		if net.Name == netName || net.ID == netName {
			if networkMatchFound {
				_, err = apiClient.NetworkRemove(ctx, net.ID, client.NetworkRemoveOptions{})

				if err != nil && errdefs.IsNotFound(err) {
					util.WarningMessage(fmt.Sprintf("Unable to remove network (%s) Err: %v", netName, err))
					return
				}
			} else {
				networkMatchFound = true
			}
		}
	}
}

// IsErrNotFound returns true if the error is a NotFound error, which is returned
// by the API when some object is not found. It is an alias for [cerrdefs.IsNotFound].
// Used as a wrapper to avoid direct import for docker client.
func IsErrNotFound(err error) bool {
	return errdefs.IsNotFound(err)
}
