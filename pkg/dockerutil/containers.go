package dockerutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/namnh198/lodev/pkg/archive"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// NoHealthCheck is a HealthConfig that disables any existing healthcheck when
// running a container. Used by RunSimpleContainer
// See https://pkg.go.dev/github.com/moby/docker-image-spec/specs-go/v1#HealthcheckConfig
var NoHealthCheck = container.HealthConfig{
	Test: []string{"NONE"}, // Disables any existing health check
}

// containerUser holds the UID, GID, and username used to run containers
type ContainerUser struct {
	uidStr   string
	gidStr   string
	username string
}

var (
	// sContainerUser is the singleton instance of ContainerUser
	sContainerUser *ContainerUser
	// sContainerUserOnce ensures sContainerUser is initialized only once
	sContainerUserOnce sync.Once
)

// sanitizeUsername converts a username to be safe for Linux containers.
// Linux usernames can only contain: a-z, 0-9, _, -
// and must start with a letter.
func sanitizeUsername(rawUsername string) string {
	username := rawUsername

	// Normalize unicode characters (remove diacritics)
	// Per https://stackoverflow.com/a/65981868/215713
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	username, _, _ = transform.String(t, username)

	// Handle Windows domain\user format - extract username after backslash
	if idx := strings.LastIndex(username, `\`); idx >= 0 {
		username = username[idx+1:]
	}

	// Lowercase and remove all invalid characters
	// Linux usernames can only contain: a-z, 0-9, _, -
	username = strings.ToLower(username)
	username = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return -1 // Remove character
	}, username)

	if len(username) == 0 || !util.IsLetter(string(username[0])) {
		username = "a" + username
	}

	return username
}

// GetContainerUser returns the uid, gid, and username used to run most containers
func GetContainerUser() (uidStr string, gidStr string, username string) {
	sContainerUserOnce.Do(func() {
		// Default fallback values if we can't determine the user
		uidStr = "1000"
		gidStr = "1000"
		username = "lodev"

		curUser, err := user.Current()
		if err != nil {
			// Use fallback values and warn
			util.Warning("Unable to determine current user (UID, GID, username), using fallback uid=%s gid=%s username=%s: %v", uidStr, gidStr, username, err)
		} else {
			// Use actual user values
			uidStr = curUser.Uid
			gidStr = curUser.Gid
			username = curUser.Username

			// Sanitize username for safe use in Linux containers
			// Example problem usernames: "André Kraus", "Mück", "DOMAIN\user", "user@example.com"
			// See https://stackoverflow.com/questions/64933879
			username = sanitizeUsername(username)
		}

		// Windows user IDs are non-numeric,
		// so we have to run as arbitrary user 1000. We may have a host uidStr/gidStr greater in other contexts,
		// 1000 seems not to cause file permissions issues at least on docker-for-windows.
		if util.IsWindows() {
			uidStr = "1000"
			gidStr = "1000"
		}
		sContainerUser = &ContainerUser{
			uidStr:   uidStr,
			gidStr:   gidStr,
			username: username,
		}
	})

	return sContainerUser.uidStr, sContainerUser.gidStr, sContainerUser.username
}

// ContainerName returns the container's human-readable name.
func ContainerName(c *container.Summary) string {
	if len(c.Names) == 0 {
		return c.ID
	}
	return c.Names[0][1:]
}

// TruncateID returns a shorthand version of a string identifier for convenience.
// This is a copy from https://github.com/moby/moby/blob/master/client/pkg/stringid/stringid.go
func TruncateID(id string) string {
	if i := strings.IndexRune(id, ':'); i >= 0 {
		id = id[i+1:]
	}
	shortLen := 12
	if len(id) > shortLen {
		id = id[:shortLen]
	}
	return id
}

// GetContainerNames takes an array of Container
// and returns an array of strings with container names.
// Use removePrefix to get short container names.
func GetContainerNames(containers []container.Summary, excludeContainerNames []string, removePrefix string) []string {
	var names []string
	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := c.Names[0][1:] // Trimming the leading '/' from the container name
		if slices.Contains(excludeContainerNames, name) {
			continue
		}
		if removePrefix != "" {
			name = strings.TrimPrefix(name, removePrefix)
		}
		names = append(names, name)
	}
	return names
}

// ContainerNames takes an array of Container
// and returns an array of strings with container names.
// Use removePrefix to get short container names.
func ContainerNames(containers []container.Summary, excludeContainerNames []string, removePrefix string) []string {
	var names []string
	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := c.Names[0][1:] // Trimming the leading '/' from the container name
		if slices.Contains(excludeContainerNames, name) {
			continue
		}
		if removePrefix != "" {
			name = strings.TrimPrefix(name, removePrefix)
		}
		names = append(names, name)
	}
	return names
}

// FindContainerByName takes a container name and returns the container
// If container is not found, returns nil with no error
func FindContainerByName(name string) (*container.Summary, error) {
	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return nil, err
	}

	containers, err := apiClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: client.Filters{}.Add("name", name),
	})
	if err != nil {
		return nil, err
	}
	if len(containers.Items) == 0 {
		return nil, nil
	}

	// ListContainers can return partial matches. Make sure we only match the exact one
	// we're after.
	for _, c := range containers.Items {
		if len(c.Names) > 0 && c.Names[0] == "/"+name {
			return &c, nil
		}
	}
	return nil, nil
}

// GetContainerStateByName returns container state for the named container
func GetContainerStateByName(name string) (container.ContainerState, error) {
	c, err := FindContainerByName(name)
	if err != nil || c == nil {
		return "doesnotexist", fmt.Errorf("container %s does not exist", name)
	}
	if c.State == container.StateRunning {
		return c.State, nil
	}
	return c.State, fmt.Errorf("container %s is in state=%s so can't be accessed", name, c.State)
}

// CopyIntoContainer copies a path (file or directory) into a specified container and location
func CopyIntoContainer(srcPath string, containerName string, dstPath string, exclusion string) error {
	startTime := time.Now()
	fi, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	// If a file has been passed in, we'll copy it into a temp directory
	if !fi.IsDir() {
		dirName, err := os.MkdirTemp("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dirName)
		err = fileutil.CopyFile(srcPath, filepath.Join(dirName, filepath.Base(srcPath)))
		if err != nil {
			return err
		}
		srcPath = dirName
	}

	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return err
	}
	cid, err := FindContainerByName(containerName)
	if err != nil {
		return err
	}
	if cid == nil {
		return fmt.Errorf("copyIntoContainer unable to find a container named %s", containerName)
	}

	uid, _, _ := GetContainerUser()
	_, stderr, err := Exec(cid.ID, "mkdir -p "+dstPath, uid)
	if err != nil {
		return fmt.Errorf("unable to mkdir -p %s inside %s: %v (stderr=%s)", dstPath, containerName, err, stderr)
	}

	tarball, err := os.CreateTemp(os.TempDir(), "containercopytmp*.tar.gz")
	if err != nil {
		return err
	}
	err = tarball.Close()
	if err != nil {
		return err
	}
	// nolint: errcheck
	defer os.Remove(tarball.Name())

	// Tar up the source directory into the tarball
	err = archive.Tar(srcPath, tarball.Name(), exclusion)
	if err != nil {
		return err
	}
	t, err := os.Open(tarball.Name())
	if err != nil {
		return err
	}

	// nolint: errcheck
	defer t.Close()

	_, err = apiClient.CopyToContainer(ctx, cid.ID, client.CopyToContainerOptions{DestinationPath: dstPath, Content: t, AllowOverwriteDirWithFile: true})
	if err != nil {
		return err
	}

	util.Debug("Copied %s:%s into %s in %v", srcPath, containerName, dstPath, time.Since(startTime))
	return nil
}

// CopyFromContainer copies a path from a specified container and location to a dstPath on host
func CopyFromContainer(containerName string, containerPath string, hostPath string) error {
	startTime := time.Now()
	err := os.MkdirAll(hostPath, 0755)
	if err != nil {
		return err
	}

	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return err
	}
	cid, err := FindContainerByName(containerName)
	if err != nil {
		return err
	}
	if cid == nil {
		return fmt.Errorf("copyFromContainer unable to find a container named %s", containerName)
	}

	f, err := os.CreateTemp("", filepath.Base(hostPath)+".tar.gz")
	if err != nil {
		return err
	}
	//nolint: errcheck
	defer f.Close()
	//nolint: errcheck
	defer os.Remove(f.Name())
	// nolint: errcheck

	reader, err := apiClient.CopyFromContainer(ctx, cid.ID, client.CopyFromContainerOptions{SourcePath: containerPath})
	if err != nil {
		return err
	}

	defer reader.Content.Close()

	_, err = io.Copy(f, reader.Content)
	if err != nil {
		return err
	}

	err = f.Close()
	if err != nil {
		return err
	}

	err = archive.Untar(f.Name(), hostPath, "")
	if err != nil {
		return err
	}
	util.Success("Copied %s:%s to %s in %v", containerName, containerPath, hostPath, time.Since(startTime))

	return nil
}

// FindContainersByLabels takes a map of label names and values and returns any Docker containers which match all labels.
// Explanation of the query:
// * docs: https://docs.docker.com/engine/api/v1.23/
// * Stack Overflow: https://stackoverflow.com/questions/28054203/docker-remote-api-filter-exited
func FindContainersByLabels(labels map[string]string) ([]container.Summary, error) {
	if len(labels) == 0 {
		return nil, fmt.Errorf("The provided list of label is empty")
	}

	filterList := client.Filters{}

	for key, value := range labels {
		label := fmt.Sprintf("%s=%s", key, value)

		if value == "" {
			label = key
		}
		filterList = filterList.Add("label", label)
	}

	ctx, apiClient, err := GetDockerClient()

	if err != nil {
		return nil, err
	}

	containers, err := apiClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: filterList,
	})

	if err != nil {
		return nil, err
	}

	return containers.Items, nil
}

// FindContainerByLabels takes a map of label names and values and returns any Docker containers which match all labels.
func FindContainerByLabels(labels map[string]string) (*container.Summary, error) {
	containers, err := FindContainersByLabels(labels)
	if err != nil {
		return nil, err
	}
	if len(containers) > 0 {
		return &containers[0], nil
	}
	return nil, nil
}

// ContainerWait provides a wait loop to check for a single container in "healthy" status.
// waittime is in seconds.
// This is modeled on https://gist.github.com/ngauthier/d6e6f80ce977bedca601
// Returns logoutput, error, returns error if not "healthy"
func ContainerWait(waittime int, labels map[string]string) (string, error) {
	durationWait := time.Duration(waittime) * time.Second
	timeoutChan := time.NewTimer(durationWait)
	tickChan := time.NewTicker(500 * time.Millisecond)
	defer tickChan.Stop()
	defer timeoutChan.Stop()

	status := ""
	lastStatus := ""
	startTime := time.Now()
	lastLogTime := startTime

	for {
		select {
		case <-timeoutChan.C:
			_ = timeoutChan.Stop()
			desc := ""
			c, err := FindContainerByLabels(labels)
			if err == nil && c != nil {
				health, _ := GetContainerHealth(c)
				if health != string(container.Healthy) {
					name, suggestedCommand := getSuggestedCommandForContainerLog(c, waittime)
					desc = desc + fmt.Sprintf(" %s:%s\n%s", name, health, suggestedCommand)
				}
			}
			return "", fmt.Errorf("health check timed out after %v: labels %v timed out without becoming healthy, status=%v, detail=%s ", durationWait, labels, status, desc)

		case <-tickChan.C:
			c, err := FindContainerByLabels(labels)
			cName := ""
			if err != nil || c == nil {
				return "", fmt.Errorf("failed to query container %s labels=%v: %v", cName, labels, err)
			}
			if len(c.Names) > 0 {
				cName = strings.TrimPrefix(c.Names[0], "/")
			}
			health, logOutput := GetContainerHealth(c)

			// Log status changes and periodic updates under LODEV_DEBUG
			elapsed := time.Since(startTime).Round(time.Millisecond)
			if health != lastStatus {
				util.Debug("ContainerWait: %s status change: '%s' after %v", cName, health, elapsed)
				lastStatus = health
				lastLogTime = time.Now()
			} else if time.Since(lastLogTime) >= 5*time.Second {
				util.Debug("ContainerWait: still waiting for %s, status='%s' after %v", cName, health, elapsed)
				lastLogTime = time.Now()
			}

			switch health {
			case string(container.Healthy):
				return logOutput, nil
			case string(container.Unhealthy):
				name, suggestedCommand := getSuggestedCommandForContainerLog(c, 0)
				return logOutput, fmt.Errorf("%s container is unhealthy, log=%s\n%s", name, logOutput, suggestedCommand)
			case string(container.StateExited):
				name, suggestedCommand := getSuggestedCommandForContainerLog(c, 0)
				return logOutput, fmt.Errorf("%s container exited,\n%s", name, suggestedCommand)
			}
		}
	}
}

// ContainersWait provides a wait loop to check for multiple containers in "healthy" status.
// waittime is in seconds.
// filterServices optionally limits which containers are checked by their
// "com.docker.compose.service" label. When empty, all containers matching
// labels are checked.
// Returns error if not all containers become "healthy" before the timeout.
func ContainersWait(waittime int, labels map[string]string, filterServices ...string) error {
	timeoutChan := time.After(time.Duration(waittime) * time.Second)
	tickChan := time.NewTicker(500 * time.Millisecond)
	defer tickChan.Stop()

	status := ""
	lastStatus := ""
	startTime := time.Now()
	lastLogTime := startTime

	for {
		select {
		case <-timeoutChan:
			desc := ""
			containers, err := FindContainersByLabels(labels)
			if err == nil && containers != nil {
				for _, c := range containers {
					if len(filterServices) > 0 && !slices.Contains(filterServices, c.Labels["com.docker.compose.service"]) {
						continue
					}
					health, _ := GetContainerHealth(&c)
					if health != string(container.Healthy) {
						name, suggestedCommand := getSuggestedCommandForContainerLog(&c, waittime)
						desc = desc + fmt.Sprintf(" %s:%s\n%s", name, health, suggestedCommand)
					}
				}
			}
			return fmt.Errorf("health check timed out: labels %v timed out without becoming healthy, status=%v, detail=%s ", labels, status, desc)

		case <-tickChan.C:
			containers, err := FindContainersByLabels(labels)
			if err != nil || containers == nil {
				return fmt.Errorf("failed to query container labels=%v: %v", labels, err)
			}
			allHealthy := true
			healthyCount := 0
			totalCount := 0
			for _, c := range containers {
				if len(filterServices) > 0 && !slices.Contains(filterServices, c.Labels["com.docker.compose.service"]) {
					continue
				}
				totalCount++
				health, logOutput := GetContainerHealth(&c)

				switch health {
				case string(container.Healthy):
					healthyCount++
					continue
				case string(container.Unhealthy):
					name, suggestedCommand := getSuggestedCommandForContainerLog(&c, 0)
					return fmt.Errorf("%s container is unhealthy, log=%s\n%s", name, logOutput, suggestedCommand)
				case string(container.StateExited):
					name, suggestedCommand := getSuggestedCommandForContainerLog(&c, 0)
					return fmt.Errorf("%s container exited.\n%s", name, suggestedCommand)
				default:
					allHealthy = false
				}
			}

			// Ensure every requested service has at least one container.
			// Without this, we could return success before a service
			// (e.g. db) has registered with the Docker API.
			if len(filterServices) > 0 {
				for _, svc := range filterServices {
					if !slices.ContainsFunc(containers, func(c container.Summary) bool {
						return c.Labels["com.docker.compose.service"] == svc
					}) {
						allHealthy = false
						break
					}
				}
			} else if totalCount == 0 {
				allHealthy = false
			}

			// Log status changes and periodic updates under LODEV_DEBUG
			currentStatus := fmt.Sprintf("%d/%d healthy", healthyCount, totalCount)
			elapsed := time.Since(startTime).Round(time.Millisecond)
			if currentStatus != lastStatus {
				util.Debug("ContainersWait: status changed to '%s' after %v", currentStatus, elapsed)
				lastStatus = currentStatus
				lastLogTime = time.Now()
			} else if time.Since(lastLogTime) >= 5*time.Second {
				util.Debug("ContainersWait: still waiting, status='%s' after %v", currentStatus, elapsed)
				lastLogTime = time.Now()
			}

			if allHealthy {
				return nil
			}
		}
	}
}

// GetPublishedPort returns the published port for a given private port.
func GetPublishedPort(privatePort uint16, c container.Summary) int {
	for _, port := range c.Ports {
		if port.PrivatePort == privatePort {
			return int(port.PublicPort)
		}
	}
	return 0
}

// getSuggestedCommandForContainerLog returns a command that can be used to find out what is wrong with a container
func getSuggestedCommandForContainerLog(c *container.Summary, timeout int) (string, string) {
	var suggestedCommands []string
	service := c.Labels["com.docker.compose.service"]
	if service != "" && service != "lodev-router" {
		suggestedCommands = append(suggestedCommands, fmt.Sprintf("lodev log -s %s", service))
	}
	name := ContainerName(c)
	suggestedCommands = append(suggestedCommands, fmt.Sprintf("docker logs %s", name), fmt.Sprintf("docker inspect --format \"{{ json .State.Health }}\" %s | docker run -i --rm namnh198/lodev-utilities jq -r", name))
	troubleshootingCommand, _ := util.ArrayToReadableOutput(suggestedCommands)
	suggestedCommand := "\nTroubleshoot this with these commands:\n" + troubleshootingCommand
	if timeout > 0 && service != "lodev-router" {
		timeoutNote := "\nIf your internet connection is slow, consider increasing the timeout by running this:\n"
		timeoutCommand, _ := util.ArrayToReadableOutput([]string{fmt.Sprintf("lodev config --default-container-timeout=%d && lodev restart", timeout*2)})
		suggestedCommand = suggestedCommand + timeoutNote + timeoutCommand
	}
	if util.IsVerbose() {
		ctx, apiClient, err := GetDockerClient()
		if err == nil {
			var stdout bytes.Buffer
			logOpts := client.ContainerLogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Follow:     false,
				Timestamps: false,
			}
			rc, err := apiClient.ContainerLogs(ctx, c.ID, logOpts)
			if err != nil {
				util.Warning("Unable to capture logs from %s container: %v", name, err)
			} else {
				defer rc.Close()
				_, err = stdcopy.StdCopy(&stdout, &stdout, rc)
				if err != nil {
					util.Warning("Unable to copy logs from %s container: %v", name, err)
				}
				util.Debug("Logs from failed %s container:\n%s\n", name, strings.TrimSpace(stdout.String()))
			}
			_, logOutput := GetContainerHealth(c)
			util.Debug("Health log from failed %s container:\n%s\n", name, strings.TrimSpace(logOutput))
		}
	}
	return name, suggestedCommand
}

// GetContainerHealth retrieves the health status of a given container.
// returns status, most-recent-log
// The container is only considered "healthy" if it's also "running", contrary to Docker's normal usage
func GetContainerHealth(c *container.Summary) (string, string) {
	if c == nil {
		return "stop", ""
	}
	cState := string(c.State)

	// if the container is not running, returns exited as health status
	if cState == string(container.StateExited) || cState == string(container.StateRestarting) {
		return cState, ""
	}

	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return "", ""
	}

	inspect, err := apiClient.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
	if err != nil {
		util.Warning("Error getting container to inspect: %v", err)
		return "", ""
	}
	logOuput := ""
	status := ""

	if inspect.Container.State.Health != nil {
		status = string(inspect.Container.State.Health.Status)
	}

	if status != "" {
		numLogs := len(inspect.Container.State.Health.Log)
		if numLogs > 0 {
			logOuput = fmt.Sprintf("%v", inspect.Container.State.Health.Log[numLogs-1].Output)
		}
		// A container can't be healthy if it's not running
		// Docker/Podman may cache the last health status even after state changes.
		if inspect.Container.State.Status != container.StateRunning {
			switch inspect.Container.State.Status {
			case container.StateExited, container.StateRestarting:
				status = string(inspect.Container.State.Status)
			default:
				status = string(container.Unhealthy)
			}
		}
	} else {
		// Some container's may not have a healthcheck, in that case we use determine health by the state of the container
		switch inspect.Container.State.Status {
		case container.StateRunning:
			status = string(container.Healthy)
		default:
			status = string(container.Unhealthy)
		}
	}

	return status, strings.TrimSpace(logOuput)
}

// GetBoundHostPorts takes a container pointer and returns an array
// of exposed ports (and error)
func GetBoundHostPorts(containerID string) ([]string, error) {
	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return nil, err
	}
	inspectInfo, err := apiClient.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})

	if err != nil {
		return nil, err
	}

	portMap := map[string]bool{}

	if inspectInfo.Container.HostConfig != nil && inspectInfo.Container.HostConfig.PortBindings != nil {
		for _, portBindings := range inspectInfo.Container.HostConfig.PortBindings {
			if len(portBindings) > 0 {
				for _, binding := range portBindings {
					// Only include ports with a non-empty HostPort
					if binding.HostPort != "" {
						portMap[binding.HostPort] = true
					}
				}
			}
		}
	}
	var ports []string
	for k := range portMap {
		ports = append(ports, k)
	}
	slices.Sort(ports)
	return ports, nil
}

// GetRouterNetworkAliases takes a container ID and returns network aliases
// from the LODEV_default network (and error)
func GetRouterNetworkAliases(containerID string) ([]string, error) {
	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return nil, err
	}

	inspectInfo, err := apiClient.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, err
	}

	aliasMap := map[string]bool{}

	// Extract aliases from LODEV_default network
	if inspectInfo.Container.NetworkSettings != nil && inspectInfo.Container.NetworkSettings.Networks != nil {
		if lodevNetwork, ok := inspectInfo.Container.NetworkSettings.Networks[nodeps.LodevNetwork]; ok {
			if lodevNetwork.Aliases != nil {
				for _, alias := range lodevNetwork.Aliases {
					// Filter out the router's own container name and ID
					// Docker automatically adds these, but we only want project hostnames
					if alias != nodeps.RouterContainer && alias != containerID[:12] {
						aliasMap[alias] = true
					}
				}
			}
		}
	}

	aliases := []string{}
	for k := range aliasMap {
		aliases = append(aliases, k)
	}
	slices.Sort(aliases)
	return aliases, nil
}

// Exec does a simple docker exec, no frills, it executes the command
// with the specified uid (or defaults to root=0 if empty uid)
// Returns stdout, stderr, error
func Exec(containerID string, command string, uid string) (string, string, error) {
	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return "", "", err
	}

	if uid == "" {
		uid = "0"
	}
	execCreate, err := apiClient.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		Cmd:          []string{"sh", "-c", command},
		AttachStdout: true,
		AttachStderr: true,
		User:         uid,
	})
	if err != nil {
		return "", "", err
	}

	var stdout, stderr bytes.Buffer
	execAttach, err := apiClient.ExecAttach(ctx, execCreate.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", "", err
	}
	defer execAttach.Close()

	_, err = stdcopy.StdCopy(&stdout, &stderr, execAttach.Reader)
	if err != nil {
		return "", "", err
	}

	info, err := apiClient.ExecInspect(ctx, execCreate.ID, client.ExecInspectOptions{})
	if err != nil {
		return stdout.String(), stderr.String(), err
	}
	var execErr error
	if info.ExitCode != 0 {
		execErr = fmt.Errorf("command '%s' returned exit code %v", command, info.ExitCode)
	}

	return stdout.String(), stderr.String(), execErr
}

// GetContainerEnv returns the value of a given environment variable from a given container.
func GetContainerEnv(key string, c container.Summary) string {
	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return ""
	}

	inspect, err := apiClient.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
	if err != nil {
		return ""
	}

	envVars := inspect.Container.Config.Env

	// envVars is a list of string format KEY=VALUE
	for _, env := range envVars {
		if strings.HasPrefix(env, key) {
			return strings.TrimPrefix(env, key+"=")
		}
	}

	return ""
}

// GetPublishedPort returns the published port for a given private port
func GetPublisedPort(privatePort uint16, c container.Summary) int {
	for _, port := range c.Ports {
		if port.PrivatePort == privatePort {
			return int(port.PublicPort)
		}
	}
	return 0
}

// RemoveContainer forced to remove a container by its ID
func RemoveContainer(cID string) error {
	ctx, apiClient, err := GetDockerClient()
	if err != nil {
		return err
	}

	_, err = apiClient.ContainerRemove(ctx, cID, client.ContainerRemoveOptions{Force: true})
	return err
}

// RemoveContainerByLabels stops and removes a container by its labels
func RemoveContainerByLabels(labels map[string]string) error {
	ctx, apitClient, err := GetDockerClient()
	if err != nil {
		return err
	}

	containers, err := FindContainersByLabels(labels)
	if err != nil {
		return err
	}

	if containers == nil {
		return nil
	}

	for _, c := range containers {
		_, err = apitClient.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true})
		if err != nil {
			return err
		}
	}

	return nil
}
