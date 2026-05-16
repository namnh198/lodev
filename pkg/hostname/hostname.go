package hostname

import (
	"fmt"

	goodhosts "github.com/goodhosts/hostsfile"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/util"
)

// LodevHosts is a wrapper around goodhosts.Hosts to manage host entries for lodev projects
type LodevHosts struct {
	*goodhosts.Hosts
}

const WSL2WindowsHostsFile = `/mnt/c/Windows/system32/drivers/etc/hosts`
const WindowHostsFile = `C:\Windows\System32\drivers\etc\hosts`
const DefaultHostsFile = `/etc/hosts`

// const Host

// New is a simple wrapper on goodhosts.NewHosts()
func New() (*LodevHosts, error) {
	var hosts *goodhosts.Hosts
	var err error

	if util.IsWSL2() {
		hosts, err = goodhosts.NewCustomHosts(WSL2WindowsHostsFile)
	} else {
		hosts, err = goodhosts.NewHosts()
	}

	return &LodevHosts{hosts}, err
}

// AddHost adds an entry to default hosts file if it doesn’t already exist
func (h *LodevHosts) AddHost(hostname, ip string) error {
	if h.Hosts.Has(ip, hostname) {
		return nil
	}
	err := h.Hosts.Add(ip, hostname)
	if err != nil {
		return err
	}

	err = h.Hosts.Flush()

	if err != nil {
		return fmt.Errorf("Failed to add host %s: %v\nTroubleshooting: ", hostname, err)
	}

	return nil
}

// RemoveHost removes named entry if it exists
func (h *LodevHosts) RemoveHost(hostname, ip string) error {
	if !h.Hosts.Has(ip, hostname) {
		return nil
	}
	err := h.Hosts.Remove(ip, hostname)
	if err != nil {
		return err
	}
	err = h.Hosts.Flush()

	if err != nil {
		return fmt.Errorf("Failed to remove host %s: %v\nTroubleshooting: ", hostname, err)
	}

	return nil
}

type LodevHostName struct {
	Host string
	IP   string
}

// GetDockerIP returns the Docker IP address, which is used for host entries in /etc/hosts
func getDockerIP() (string, error) {
	dockerIP, err := dockerutil.GetDockerIP()
	if err != nil {
		return "", fmt.Errorf("Failed to get Docker IP: %v", err)
	}

	// On remote Docker hosts, the Docker IP (e.g. a cloud provider's public IP)
	// is not a valid bind address on the Docker host itself, so bind to all interfaces.
	if dockerutil.IsRemoteDockerHost() {
		dockerIP = "0.0.0.0"
	}
	return dockerIP, nil
}

// AddHostsIfNeeded will run to add the hosts to /etc/hosts.
func AddHostsIfNeeded(hostsnames []string) error {
	host, err := New()
	if err != nil {
		return err
	}
	dockerIP, err := getDockerIP()
	if err != nil {
		return err
	}

	// Add new hostnames
	for _, hostname := range hostsnames {
		err = host.Hosts.Add(dockerIP, hostname)
		if err != nil {
			return err
		}
	}

	if err = host.Hosts.Flush(); err != nil {
		return fmt.Errorf("Failed to add hosts: %v\nTroubleshooting: ", err)
	}

	return nil
}

// AddHostsIfNeeded will run to add the hosts to /etc/hosts.
func RemoveHostsIfNeeded(hostsnames []string) error {
	host, err := New()
	if err != nil {
		return err
	}

	dockerIP, err := getDockerIP()
	if err != nil {
		return err
	}

	// Remove existing hostnames that are no longer needed
	for _, hostname := range hostsnames {
		_ = host.Hosts.Remove(dockerIP, hostname)
	}

	if err = host.Hosts.Flush(); err != nil {
		return fmt.Errorf("Failed to add hosts: %v\nTroubleshooting: ", err)
	}

	return nil
}
