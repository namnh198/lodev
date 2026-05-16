package lodev

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/moby/moby/api/types/container"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/netutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/yarlson/tap"
)

// EphemeralRouterPortsAssigned is used when we have assigned an ephemeral port
// but it may not yet be occupied. A map is used just to make it easy
// to detect if it's there, the value in the map is not used.
var EphemeralRouterPortsAssigned = make(map[int]bool)

// SortWebExtraExposedPorts sorts the WebExtraExposedPorts slice based on how well the ports match the requested HTTP and HTTPS ports in the global config.
func (p *Project) SortWebExtraExposedPorts() {
	if len(p.WebExtraExposedPorts) <= 1 {
		return
	}
	httpPort, _ := strconv.Atoi(LodevConfig.HttpPort)
	httpsPort, _ := strconv.Atoi(LodevConfig.HttpsPort)

	// Sort ports so the best match for the requested HTTP/HTTPS ports comes first.
	// The order is stable, so ports with the same match quality keep their
	// original relative order.
	slices.SortStableFunc(p.WebExtraExposedPorts, func(a, b ProjectWebExtraExposedPorts) int {
		aMatch := 0
		bMatch := 0

		// Match scoring:
		// 2 = both HTTP and HTTPS ports match exactly
		// 1 = either HTTP or HTTPS port matches
		// 0 = no match at all
		if a.HTTPPort == httpPort && a.HTTPSPort == httpsPort {
			aMatch = 2
		} else if a.HTTPPort == httpPort || a.HTTPSPort == httpsPort {
			aMatch = 1
		}
		if b.HTTPPort == httpPort && b.HTTPSPort == httpsPort {
			bMatch = 2
		} else if b.HTTPPort == httpPort || b.HTTPSPort == httpsPort {
			bMatch = 1
		}
		// Sort in descending order so higher-quality matches appear first
		return bMatch - aMatch
	})
}

// GetRouterComposeYAMLPath returns the path to the router compose YAML file in the global .lodev directory
func RouterComposeYAMLPath() string {
	return GetLodevConfigPath(".router-compose.yaml")
}

// IsRouterDisabled returns true if the router is disabled (e.g. when running in a devcontainer), false otherwise
func IsRouterDisabled() bool {
	if util.IsDevcontainer() {
		return true
	}

	return false
}

// StartLodevRouter ensure lodev-router is running, and if not try to start it
func StartLodevRouter(needsRecreation bool) error {
	if IsRouterDisabled() {
		return nil
	}

	router, err := FindLodevRouter()

	if err == nil && router != nil && router.State != "running" {
		// Kill the router if not running or if its image is outdated, so it restarts fresh.
		// Use HasSuffix to handle registry prefixes (e.g. docker.io/) that Podman includes in image names.
		err = dockerutil.RemoveContainer(nodeps.RouterContainer)
		if err != nil {
			return err
		}
		router = nil
	}

	// Check if router needs to be recreated due to port or hostname changes
	if router != nil && err == nil && router.State == "running" {
		// Router is running, check if ports or hostnames have changed
		var portsChanged, hostnamesChanged bool
		// Check ports
		existingPorts, err := dockerutil.GetBoundHostPorts(router.ID)
		if err != nil {
			util.Debug("Error getting bound ports, will recreate router: %v", err)
			needsRecreation = true
		} else if !needsRecreation {
			neededPorts := determineRouterPorts()
			neededPorts = append(neededPorts, []string{LodevConfig.HttpPort, LodevConfig.HttpsPort, LodevConfig.TraefikMonitorPort}...)
			portsChanged = !PortsMatch(existingPorts, neededPorts)
			util.Debug("Router port comparison: existing=%v needed=%v changed=%v", existingPorts, neededPorts, portsChanged)
			// Check hostnames (network aliases)
			existingHostnames, err := dockerutil.GetRouterNetworkAliases(router.ID)
			if err != nil {
				util.Debug("Error getting network aliases, will recreate router: %v", err)
				needsRecreation = true
			} else {
				neededHostnames := determineRouterHostnames()
				hostnamesChanged = !HostnamesMatch(existingHostnames, neededHostnames)
				util.Debug("Router hostname comparison: existing=%v needed=%v changed=%v", existingHostnames, neededHostnames, hostnamesChanged)
			}

			if portsChanged || hostnamesChanged {
				if portsChanged && hostnamesChanged {
					util.Debug("Router ports and hostnames have changed, will recreate router")
				} else if portsChanged {
					util.Debug("Router ports have changed, will recreate router")
				} else {
					util.Debug("Router hostnames have changed, will recreate router")
				}
				needsRecreation = true
			} else {
				util.Debug("Router ports and hostnames have not changed, skipping recreation")
			}
		}
	} else {
		// Router is not running, needs to be started
		needsRecreation = true
	}

	if needsRecreation {
		err = StopLodevRouter()
		if err != nil {
			return fmt.Errorf("failed to stop existing router: %v", err)
		}
		wait := util.StartWait("Start LODEV router")
		routerComposeFullPath, err := generateRouterCompose()
		if err != nil {
			return err
		}

		err = pushGlobalTraefikConfig()
		if err != nil {
			return fmt.Errorf("failed to push global Traefik config: %v", err)
		}

		err = CheckRouterPorts()
		if err != nil {
			return fmt.Errorf("unable to listen on required ports. err=%v", err)
		}

		// Run docker-compose up -d against the lodev-router full compose file
		_, err = ComposeCmd(&ComposeCmdOpts{
			ProjectName:  nodeps.RouterComposeProjectName,
			ComposeFiles: []string{routerComposeFullPath},
			Progress:     wait.Progress,
			Action:       []string{"up", "--build", "-d"},
		})

		elapsed := wait.Complete(err, "LODEV router started")
		if err != nil {
			return fmt.Errorf("failed to start %s in %.1fs; err=%v", nodeps.RouterContainer, elapsed.Seconds(), err)
		}

		// Normally the router comes right up, but when
		// it has to do let's encrypt updates, it can take some time.
		routerWaitTimeout := 120
		if LodevConfig.UseLetsEncrypt {
			routerWaitTimeout = 180
		}

		label := map[string]string{
			"com.docker.compose.service": nodeps.RouterContainer,
			"com.docker.compose.oneoff":  "False",
		}
		waitRouterSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
		waitRouterSpin.Start(fmt.Sprintf("Waiting for %s to become ready", nodeps.RouterContainer))
		util.Debug("Router wait: checking for container with labels %v, timeout %d seconds, polling every 500ms for healthy status", label, routerWaitTimeout)
		logOutput, err := dockerutil.ContainerWait(routerWaitTimeout, label)
		if err != nil {
			err = fmt.Errorf("lodev-router failed to become ready after %ds; log=%s, err=%v", routerWaitTimeout, logOutput, err)
			waitRouterSpin.Stop(err.Error(), 2)
			return err
		}
		waitRouterSpin.Stop("lodev-router is ready", 0)
	} else {
		traefigConfigSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
		traefigConfigSpin.Start("Router already running, updating Traefik configuration")
		// Even if we don't recreate, update the Traefik config for the new project
		err = pushGlobalTraefikConfig()
		if err != nil {
			err = fmt.Errorf("failed to push global Traefik config: %v", err)
			traefigConfigSpin.Stop(err.Error(), 2)
			return err
		}
		traefigConfigSpin.Message("Pushed Traefik configuration")

		// Force the healthcheck to run and wait for Traefik to load the new config.
		// If this succeeds, the router is already verified healthy with the new
		// config, so we can skip the ContainerWait polling below.
		traefigConfigSpin.Message("Reset router healthy")
		err = clearRouterHealthcheck()
		if err != nil {
			err = fmt.Errorf("lodev-router failed to push config. err=%v", err)
			traefigConfigSpin.Stop(err.Error(), 2)
			return err
		}
		traefigConfigSpin.Stop("Router configuration updated", 0)
	}

	return nil
}

// StopLodevRouter stops and removes the lodev-router container if it's running
func StopLodevRouter() error {
	stopSpin := tap.NewSpinner(tap.SpinnerOptions{Indicator: "timer"})
	stopSpin.Start("Stop LODEV router")
	router, err := FindLodevRouter()
	if router == nil && err != nil {
		// Router not found, nothing to remove
		stopSpin.Stop("LODEV router not found", 2)
		return nil
	}
	err = dockerutil.RemoveContainer(nodeps.RouterContainer)
	if err != nil {
		if ok := dockerutil.IsErrNotFound(err); !ok {
			stopSpin.Stop(fmt.Sprintf("Failed to stop LODEV router: %v", err), 2)
			return err
		}
	}
	stopSpin.Stop("LODEV router stopped", 0)
	return nil
}

// generateRouterCompose() generates the ~/.lodev/.router-compose.yaml and ~/.lodev/.router-compose-full.yaml
func generateRouterCompose() (string, error) {
	exposedPorts := determineRouterPorts()

	routerComposeBasePath := GetLodevConfigPath(".router-compose.yaml")
	routerComposeFullPath := GetLodevConfigPath(".router-compose-full.yaml")

	var doc bytes.Buffer
	f, ferr := os.Create(routerComposeBasePath)
	if ferr != nil {
		return "", ferr
	}
	defer util.CheckClose(f)

	dockerIP, _ := dockerutil.GetDockerIP()
	// On remote Docker hosts, the Docker IP (e.g. a cloud provider's public IP)
	// is not a valid bind address on the Docker host itself, so bind to all interfaces.
	if dockerutil.IsRemoteDockerHost() {
		dockerIP = "0.0.0.0"
	}

	uid, gid, username := dockerutil.GetContainerUser()
	timezone, _ := GetLocalTimezone()

	templateVars := map[string]any{
		"Username":           username,
		"UID":                uid,
		"GID":                gid,
		"LodevGenerated":     nodeps.LodevFileSignature,
		"LodevDefaultVolume": fileutil.WindowsPathToCygwinPath(GetLodevConfigDir()),
		"RouterImage":        nodeps.GetRouterImage(),
		"RouterContainer":    nodeps.RouterContainer,
		"Ports":              exposedPorts,
		"DockerIP":           dockerIP,
		"UseLetsencrypt":     LodevConfig.UseLetsEncrypt,
		"LetsEncryptEmail":   LodevConfig.LetsEncryptEmail,
		"Router":             LodevConfig.Router,
		"TraefikMonitorPort": LodevConfig.TraefikMonitorPort,
		"Timezone":           timezone,
		"Hostnames":          determineRouterHostnames(),
	}

	t, err := template.New("router_compose_template.yaml").Funcs(getTemplateFuncMap()).ParseFS(bundledAssets, "router_compose_template.yaml")
	if err != nil {
		return "", err
	}

	err = t.Execute(&doc, templateVars)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(doc.String())
	if err != nil {
		return "", err
	}

	fullHandle, err := os.Create(routerComposeFullPath)
	if err != nil {
		return "", err
	}

	userFiles, err := filepath.Glob(GetLodevConfigPath(".router-compose.*.yaml"))
	if err != nil {
		return "", err
	}
	files := append([]string{RouterComposeYAMLPath()}, userFiles...)
	fullContents, err := ComposeCmd(&ComposeCmdOpts{
		ComposeFiles: files,
		Action:       []string{"config"},
	})
	if err != nil {
		return "", err
	}
	project, err := EnsureComposeYAML(fullContents.String())
	if err != nil {
		return "", err
	}
	injectLabelsComposeYAML(project, nil)
	fullContentsBytes, err := project.MarshalYAML()
	if err != nil {
		return "", err
	}
	_, err = fullHandle.Write(fullContentsBytes)
	if err != nil {
		return "", err
	}

	return routerComposeFullPath, nil
}

// FindLodevRouter uses FindContainerByLabels to get our router container and
// return it.
func FindLodevRouter() (*container.Summary, error) {
	containerQuery := map[string]string{
		"com.docker.compose.service": nodeps.RouterContainer,
		"com.docker.compose.oneoff":  "False",
	}
	c, err := dockerutil.FindContainerByLabels(containerQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute findContainersByLabels, %v", err)
	}
	if c == nil {
		return nil, fmt.Errorf("no %s was found", nodeps.RouterContainer)
	}
	return c, nil
}

// ClearRouterHealthcheck forces the router healthcheck to run immediately by
// removing the healthy marker and error file, then executing the healthcheck.
// This ensures the router status reflects the current configuration state.
func clearRouterHealthcheck() error {
	router, err := FindLodevRouter()
	if err != nil || router == nil {
		// Router not found or error - nothing to clear
		return nil
	}

	util.Debug("Forcing router healthcheck to clear status")
	uid, _, _ := dockerutil.GetContainerUser()
	_, _, err = dockerutil.Exec(router.ID, "rm -f /tmp/healthy /tmp/traefik-errors.txt && /healthcheck.sh", uid)
	if err != nil {
		return fmt.Errorf("router healthcheck failed: %v", err)
	}
	return nil
}

// determineRouterPorts returns a list of port mappings retrieved from ports from
// containers defining HTTP_EXPOSE/HTTPS_EXPOSE and VIRTUAL_PORTS env vars.
// It is only useful to call this when containers are actually running, before
// starting lodev-router (so that we can bind the port mappings needed)
func determineRouterPorts() []string {
	containers, err := getLodevContainers()

	if err != nil {
		util.ErrorMessage(fmt.Sprintf("Failed to retrieve containers for determining port mappings: %v", err))
	}

	var routerPorts []string

	for _, c := range containers {
		if _, ok := c.Labels[dockerutil.LabelAppName]; !ok {
			continue
		}

		if c.Image == nodeps.RouterImage || c.State != "running" {
			continue
		}

		var exposePorts []string

		httpPorts := dockerutil.GetContainerEnv("HTTP_EXPOSE", c)
		if httpPorts != "" {
			ports := strings.Split(httpPorts, ",")
			exposePorts = append(exposePorts, ports...)
		}

		httpsPorts := dockerutil.GetContainerEnv("HTTPS_EXPOSE", c)
		if httpsPorts != "" {
			ports := strings.Split(httpsPorts, ",")
			exposePorts = append(exposePorts, ports...)
		}

		virtualPorts := dockerutil.GetContainerEnv("VIRTUAL_PORTS", c)
		if virtualPorts != "" {
			ports := strings.Split(virtualPorts, ",")
			for _, port := range ports {
				if !regexp.MustCompile(`^[0-9]+$`).MatchString(port) {
					util.Debug("Skipping invalid VIRTUAL_PORTS entry '%s' from container %s", port, c.Names[0])
					continue
				}
				exposePorts = append(exposePorts, fmt.Sprintf("%s:80", port))
				exposePorts = append(exposePorts, fmt.Sprintf("%s:443", port))
			}
		}

		routerPorts = processExposePorts(exposePorts, routerPorts)
	}

	uniquePorts := util.SliceToUniqueSlice(&routerPorts)

	slices.Sort(uniquePorts)

	return uniquePorts
}

// determineRouterHostnames returns a list of all hostnames retrieved from ports from
// containers defining VIRTUAL_HOST and VIRTUAL_HOST env vars.
// It is only useful to call this when containers are actually running, before
// starting lodev-router (so that we can bind the port mappings needed)
func determineRouterHostnames() []string {
	containers, err := getLodevContainers()

	if err != nil {
		util.ErrorMessage(fmt.Sprintf("Failed to retrieve containers for determining port mappings: %v", err))
	}

	var hostnames []string

	for _, c := range containers {
		if _, ok := c.Labels[dockerutil.LabelAppName]; !ok {
			continue
		}

		if c.Image == nodeps.RouterImage || c.State != "running" {
			continue
		}
		virtualHost := dockerutil.GetContainerEnv("VIRTUAL_HOST", c)
		hosts := strings.Split(virtualHost, ",")
		for _, vh := range hosts {
			vh = strings.TrimSpace(vh)
			if vh != "" && IsValidHostname(vh) {
				hostnames = append(hostnames, vh)
			}
		}
		hostEnv := dockerutil.GetContainerEnv("VIRTUAL_HOST", c)
		hosts = strings.Split(hostEnv, ",")
		for _, vh := range hosts {
			vh = strings.TrimSpace(vh)
			if vh != "" && IsValidHostname(vh) {
				hostnames = append(hostnames, vh)
			}
		}
	}
	hostnames = util.SliceToUniqueSlice(&hostnames)

	return hostnames
}

// CheckRouterPorts tries to connect to the ports the router will use as a heuristic to find out
// if they're available for docker to bind to. Returns an error if either one results
// in a successful connection.
func CheckRouterPorts() error {
	routerContainer, _ := FindLodevRouter()
	var existingExposedPorts []string
	var err error
	if routerContainer != nil {
		existingExposedPorts, err = dockerutil.GetBoundHostPorts(routerContainer.ID)
		if err != nil {
			return err
		}
	}
	newRouterPorts := determineRouterPorts()

	// Check if any of the new ports are already in use
	var portError error
	for _, port := range newRouterPorts {
		if slices.Contains(existingExposedPorts, port) {
			continue
		}
		if IsPortActive(port) {
			portError = fmt.Errorf("port %s is already in use", port)
			break
		}
	}

	// If we found a port conflict, check if it might be a security software false positive
	if portError != nil {
		// If all ephemeral ports appear active, it's likely security software interference.
		// Let Docker report any real conflicts.
		// See https://github.com/lodev/lodev/issues/7921
		freePortsAvailable := false
		for p := nodeps.MinEphemeralPort; p <= nodeps.MaxEphemeralPort; p++ {
			if !IsPortActive(fmt.Sprint(p)) {
				freePortsAvailable = true
				break
			}
		}
		if !freePortsAvailable {
			util.WarningMessage("Unable to check port availability")
			util.WarningMessage("Assuming ports are available, see https://lodev.com/s/port-conflict")
			return nil
		}
		// There are free ports available, so this is a real conflict
		return portError
	}

	return nil
}

// ProcessExposePorts processes HTTP_EXPOSE and HTTPS_EXPOSE port strings and returns
// a list of external ports that need to be bound by the router.
// It handles port pair formats like "8080:80" or "8080" and validates the format.
func processExposePorts(exposePorts []string, routerPorts []string) []string {
	for _, exposePortPair := range exposePorts {
		// Ports defined as hostPort:containerPort allow for router to configure upstreams
		// for containerPort, with server listening on hostPort.
		// Exposed ports for router should be hostPort:hostPort so router
		// can determine on which port a request came in
		// and route the request to the correct upstream
		exposePort := ""
		var ports []string

		// Each port pair should be of the form <number>:<number> or <number>
		// It's possible to have received a malformed HTTP_EXPOSE or HTTPS_EXPOSE from
		// some random container, so don't break if that happens.
		if !regexp.MustCompile(`^[0-9]+(:[0-9]+)?$`).MatchString(exposePortPair) {
			continue
		}

		if strings.Contains(exposePortPair, ":") {
			ports = strings.Split(exposePortPair, ":")
		} else {
			// HTTP_EXPOSE and HTTPS_EXPOSE can be a single port, meaning port:port
			ports = []string{exposePortPair, exposePortPair}
		}
		exposePort = ports[0]

		var match bool
		for _, routerPort := range routerPorts {
			if exposePort == routerPort {
				match = true
			}
		}

		// If no match, we are adding a new port mapping
		if !match {
			routerPorts = append(routerPorts, exposePort)
		}
	}

	return routerPorts
}

// PortsMatch returns true if the existing ports contain all the needed ports.
// It's fine for the router to have extra ports bound (from other projects that have stopped),
// we only need to recreate the router when it's missing ports we need.
func PortsMatch(existingPorts, neededPorts []string) bool {
	// Create a map of existing ports for quick lookup
	existingMap := make(map[string]bool)
	for _, port := range existingPorts {
		existingMap[port] = true
	}

	// Check if all needed ports are in existing ports
	for _, port := range neededPorts {
		if !existingMap[port] {
			return false
		}
	}

	return true
}

// HostnamesMatch returns true if the existing hostnames contain all the needed hostnames.
// It's fine for the router to have extra hostnames (from other projects that have stopped),
// we only need to recreate the router when it's missing hostnames we need.
func HostnamesMatch(existingHostnames, neededHostnames []string) bool {
	// Create a map of existing hostnames for quick lookup
	existingMap := make(map[string]bool)
	for _, hostname := range existingHostnames {
		existingMap[hostname] = true
	}

	// Check if all needed hostnames are in existing hostnames
	for _, hostname := range neededHostnames {
		if !existingMap[hostname] {
			return false
		}
	}

	return true
}

// GetRouterStatus returns router status and warning if not
// running or healthy, as applicable.
// return status and most recent log
func GetRouterStatus() (string, string) {
	var status, logOutput string
	c, err := FindLodevRouter()

	if err != nil || c == nil {
		status = SiteStopped
	} else {
		status, logOutput = dockerutil.GetContainerHealth(c)
	}

	return status, logOutput
}

// AllocateAvailablePortForRouter finds an available port in the local machine, in the range provided.
// Returns the port found, and a boolean that determines if the
// port is valid (true) or not (false), and the port is marked as allocated
func AllocateAvailablePortForRouter(start, upTo int) (int, bool) {
	// Get ports already bound by the router - these can be reused
	var routerBoundPorts []string
	if router, err := FindLodevRouter(); err == nil && router != nil {
		routerBoundPorts, _ = dockerutil.GetBoundHostPorts(router.ID)
	}

	for p := start; p <= upTo; p++ {
		portStr := fmt.Sprint(p)
		// If we have already assigned this port in this session, continue looking
		if _, portAlreadyUsed := EphemeralRouterPortsAssigned[p]; portAlreadyUsed {
			continue
		}
		// If the port is already bound by the router, we can reuse it
		if slices.Contains(routerBoundPorts, portStr) {
			util.Debug("AllocateAvailablePortForRouter: port %s is already bound by router, reusing it", portStr)
			EphemeralRouterPortsAssigned[p] = true
			return p, true
		}
		// If the port is not active (available), use it
		if !netutil.IsPortActive(portStr) {
			EphemeralRouterPortsAssigned[p] = true
			return p, true
		}
	}

	return 0, false
}

// GetAvailableRouterPort gets an ephemeral replacement port when the
// proposedPort is not available.
//
// The function returns an ephemeral port if the proposedPort is bound by a process
// in the host other than the running router.
//
// Returns the original proposedPort, the ephemeral port found,
// and a bool which is true if the proposedPort has been
// replaced with an ephemeralPort
func GetAvailableRouterPort(proposedPort string, minPort, maxPort int) (string, string, bool) {
	// If the proposedPort is empty, we don't need to do anything
	if proposedPort == "" {
		return proposedPort, "", false
	}
	// If the router exists, check if it's already handling the proposedPort
	// regardless of its health status. This prevents allocating ephemeral ports
	// when the router is running but unhealthy (e.g., broken Traefik config).
	r, err := FindLodevRouter()
	if r != nil && err == nil {
		util.Debug("GetAvailableRouterPort(): Router exists, checking bound ports")
		// Check if the proposedPort is already being handled by the router.
		routerPortsAlreadyBound, err := dockerutil.GetBoundHostPorts(r.ID)
		if err != nil {
			util.Debug("GetAvailableRouterPort(): Error getting bound ports: %v", err)
			// Continue to port availability check below
		} else if slices.Contains(routerPortsAlreadyBound, proposedPort) {
			// If the proposedPort is already bound by the router,
			// there's no need to go find an ephemeral port.
			util.Debug("GetAvailableRouterPort(): proposedPort %s already bound on lodev-router, accepting it", proposedPort)
			return proposedPort, "", false
		}
	}

	// At this point, the router may or may not be running, but we
	// have not found it already having the proposedPort bound
	if !netutil.IsPortActive(proposedPort) {
		// If the proposedPort is available (not active) for use, just have the router use it
		util.Debug("GetAvailableRouterPort(): proposedPort %s is available, use proposedPort=%s", proposedPort, proposedPort)
		return proposedPort, "", false
	}

	ephemeralPort, ok := AllocateAvailablePortForRouter(minPort, maxPort)
	if !ok {
		// Unlikely, but this can happen if security software makes all ports appear active.
		util.Debug("GetAvailableRouterPort(): proposedPort %s is not available, no ephemeral ports in range %d-%d are available", proposedPort, minPort, maxPort)
		return proposedPort, "", false
	}

	util.Debug("GetAvailableRouterPort(): proposedPort %s is not available, ephemeralPort=%d is available, use it", proposedPort, ephemeralPort)

	return proposedPort, strconv.Itoa(ephemeralPort), true
}

// GetEphemeralPortsIfNeeded replaces the provided ports with an ephemeral version if they need it.
func GetEphemeralPortsIfNeeded(ports []*string, verbose bool) {
	for _, port := range ports {
		proposedPort, replacementPort, portChangeRequired := GetAvailableRouterPort(*port, nodeps.MinEphemeralPort, nodeps.MaxEphemeralPort)
		if portChangeRequired {
			*port = replacementPort
			if verbose {
				util.WarningMessage(fmt.Sprintf("Port %s is not available, using %s instead for router configuration", proposedPort, replacementPort))
			}
		}
	}
}
