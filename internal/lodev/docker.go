package lodev

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
	"github.com/yarlson/tap"
)

type ComposeCmdOpts struct {
	ComposeFiles []string
	ComposeYAML  *types.Project
	Profiles     []string
	Progress     *tap.Progress
	Action       []string
	Timeout      time.Duration
	ProjectName  string
	Env          []string
}

// ComposeWithStreams executes a docker-compose command but allows the caller to specify
// stdin/stdout/stderr
func ComposeWithStreams(cmd *ComposeCmdOpts, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	var arg []string

	_, err := DownloadDockerComposeIfNeeded()
	if err != nil {
		return err
	}

	if cmd.ProjectName != "" {
		arg = append(arg, "-p", cmd.ProjectName)
	}

	if cmd.ComposeYAML != nil {
		// Read from stdin
		arg = append(arg, "-f", "-")
	} else {
		for _, file := range cmd.ComposeFiles {
			arg = append(arg, "-f", file)
		}
	}

	arg = append(arg, cmd.Action...)

	path, err := GetDockerComposePath()
	if err != nil {
		return err
	}
	proc := exec.Command(path, arg...)
	proc.Stdout = stdout
	proc.Stderr = stderr
	if cmd.ComposeYAML != nil {
		yamlBytes, err := cmd.ComposeYAML.MarshalYAML()
		if err != nil {
			return err
		}
		yamlBytes = util.EscapeDollarSign(yamlBytes)
		proc.Stdin = strings.NewReader(string(yamlBytes))
	} else {
		proc.Stdin = stdin
	}
	proc.Env = append(os.Environ(), cmd.Env...)

	err = proc.Run()
	return err
}

// ComposeCmd executes docker-compose commands via shell.
// returns stdout, stderr, error/nil
func ComposeCmd(cmd *ComposeCmdOpts) (*bytes.Buffer, error) {
	var args []string
	var stdout bytes.Buffer
	var stderr string

	if _, err := DownloadDockerComposeIfNeeded(); err != nil {
		return &stdout, err
	}

	path, err := GetDockerComposePath()

	if err != nil {
		return &stdout, err
	}

	if cmd.ProjectName != "" {
		args = append(args, "--project-name", cmd.ProjectName)
	}

	if cmd.ComposeYAML != nil {
		args = append(args, "--file", "-")
	} else {
		for _, composeFile := range cmd.ComposeFiles {
			args = append(args, "--file", composeFile)
		}
	}

	for _, profile := range cmd.Profiles {
		args = append(args, "--profile", profile)
	}

	args = append(args, cmd.Action...)

	ctx := context.Background()

	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}

	proc := exec.CommandContext(ctx, path, args...)
	proc.Stdout = &stdout

	if cmd.ComposeYAML != nil {
		yamlBytes, err := cmd.ComposeYAML.MarshalYAML()
		if err != nil {
			return &stdout, err
		}
		yamlBytes = util.EscapeDollarSign(yamlBytes)
		proc.Stdin = strings.NewReader(string(yamlBytes))
	}

	stderrPipe, err := proc.StderrPipe()
	if err != nil {
		return &stdout, fmt.Errorf("failed to proc.StderrPipe(): %v", err)
	}

	if err = proc.Start(); err != nil {
		return &stdout, fmt.Errorf("failed to exec docker-compose: %v", err)
	}

	stderrOutput := bufio.NewScanner(stderrPipe)
	// Ignore chatty things from docker-compose like:
	// Container (or Volume) ... Creating or Created or Stopping or Starting or Removing
	// Container Stopped or Created
	// No resource found to remove (when doing a stop and no project exists)
	// ignoreRegex := "(^ *(Network|Container|Image|Volume|Service) .* (Creat|Start|Stopp|Remov|Build|Buil|Runn)(ing|t) $|.* Built$|^ *Container .*(Build|Remov)(ed|ing) *$|No services to build|Warning: No resource found to remove|Warning: Pulling fs layer|Waiting|Downloading|Extracting|Verifying Checksum|Download complete|Pull complete)"
	// downRE, err := regexp.Compile(ignoreRegex)
	// if err != nil {
	// 	util.Warning("Failed to compile regex %v: %v", ignoreRegex, err)
	// }

	for stderrOutput.Scan() {
		line := stderrOutput.Text()
		if len(stderr) > 0 {
			stderr = stderr + "\n"
		}
		stderr = stderr + line
		line = strings.Trim(line, "\n\r")

		// if downRE.MatchString(line) {
		// 	util.Debug("docker-compose output: %s", line)
		// 	continue
		// }

		if cmd.Progress != nil {
			cmd.Progress.Advance(10, line)
			time.Sleep(300 * time.Millisecond)
		}
	}

	err = proc.Wait()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &stdout, fmt.Errorf("composeCmd timed out after %v and failed to run 'COMPOSE_PROJECT_NAME=%s docker-compose %s', action='%v', err='%v', stdout='%s', stderr='%s'", cmd.Timeout, os.Getenv("COMPOSE_PROJECT_NAME"), strings.Join(args, " "), cmd.Action, err, stdout.String(), stderr)
	}

	if err != nil {
		return &stdout, fmt.Errorf("composeCmd failed to run 'COMPOSE_PROJECT_NAME=%s docker-compose %s', action='%v', err='%v', stdout='%s', stderr='%s'", os.Getenv("COMPOSE_PROJECT_NAME"), strings.Join(args, " "), cmd.Action, err, stdout.String(), stderr)
	}

	return &stdout, nil
}

// CreateComposeProject creates a compose project from a string
func EnsureComposeYAML(yamlStr string) (*types.Project, error) {
	project, err := loader.LoadWithContext(
		context.Background(),
		types.ConfigDetails{
			ConfigFiles: []types.ConfigFile{
				{Content: []byte(yamlStr)},
			},
		},
		loader.WithProfiles([]string{`*`}),
	)
	if err != nil {
		return project, err
	}
	// Initialize Networks, Services, and Volumes to empty maps if nil
	if project.Networks == nil {
		project.Networks = types.Networks{}
	}
	if project.Services == nil {
		project.Services = types.Services{}
	}
	if project.Volumes == nil {
		project.Volumes = types.Volumes{}
	}
	// Ensure nested fields like Labels, Networks, and Environment are initialized
	for name, network := range project.Networks {
		if network.Labels == nil {
			network.Labels = types.Labels{}
		}
		project.Networks[name] = network
	}
	for name, service := range project.Services {
		if service.Networks == nil {
			service.Networks = map[string]*types.ServiceNetworkConfig{}
		}
		if service.Environment == nil {
			service.Environment = types.MappingWithEquals{}
		}
		project.Services[name] = service
	}
	return project, nil
}

// PullImages pulls images in parallel if they don't exist locally
// If pullAlways is true, it will always pull
// Otherwise, it will only pull if the image doesn't exist
func PullImages(images []string, pullAlways bool, progress ...*tap.Progress) error {
	if len(images) == 0 {
		return nil
	}

	composeYamlPull, err := EnsureComposeYAML("name: compose-yaml-pull")
	if err != nil {
		return err
	}

	for _, image := range images {
		if image == "" {
			continue
		}
		if !pullAlways {
			if imageExists, _ := dockerutil.ImageExistsLocally(image); imageExists {
				continue
			}
		}
		service := sanitizeServiceName(image)
		if _, exists := composeYamlPull.Services[service]; exists {
			continue
		}
		composeYamlPull.Services[service] = types.ServiceConfig{
			Image: image,
		}
		util.Debug(`Pulling image for %s ("%s" service)`, image, service)
	}

	if len(composeYamlPull.Services) == 0 {
		util.Debug("All images already exist locally, no pull needed")
		return nil
	}
	var pullProgress *tap.Progress
	if len(progress) > 0 {
		pullProgress = progress[0]
	}

	_, err = ComposeCmd(&ComposeCmdOpts{
		ComposeYAML: composeYamlPull,
		Action:      []string{"pull"},
		Env:         []string{"COMPOSE_DISABLE_ENV_FILE=1"},
		Progress:    pullProgress,
	})

	return err
}

// Pull pulls image if it doesn't exist locally
func Pull(image string) error {
	return PullImages([]string{image}, false)
}

// FindAllImages returns an array of image tags for all containers in the compose file
func (p *Project) FindAllImages() ([]string, error) {
	var images []string
	if p.ComposeYAML == nil || p.ComposeYAML.Services == nil {
		return images, nil
	}
	for _, service := range p.ComposeYAML.Services {
		image := service.Image
		if image == "" {
			continue
		}
		if before, ok := strings.CutSuffix(image, "-built"); ok {
			image = before
			if before, ok := strings.CutSuffix(image, "-"+p.Name); ok {
				image = before
			}
		}
		images = append(images, image)
	}
	return images, nil
}

// PullBaseContainerImages pulls only the fundamentally needed images so they can be available early.
// We always need web image, and lodev-utilities for housekeeping.
func PullBaseContainerImages(additionalImages []string, pullAlways bool, progress ...*tap.Progress) error {
	base := []string{nodeps.UtilitiesImage}
	// Only pull the default web image when no project-specific images are provided,
	// otherwise the project's actual web image is already in additionalImages.
	if len(additionalImages) == 0 {
		base = append(base, nodeps.GetWebImage())
	}
	base = append(base, additionalImages...)
	return PullImages(base, pullAlways, progress...)
}

// sanitizeServiceName sanitizes a string to be a valid Docker Compose service name
// by replacing any characters that don't match [a-zA-Z0-9._-] with hyphens
// See https://github.com/compose-spec/compose-go/blob/main/schema/compose-spec.json for allowed pattern
func sanitizeServiceName(input string) string {
	if input == "" {
		return ""
	}

	invalidChars := regexp.MustCompile(`[^a-zA-Z0-9._-]`)
	sanitized := invalidChars.ReplaceAllString(input, "-")

	multipleHyphens := regexp.MustCompile(`-+`)
	sanitized = multipleHyphens.ReplaceAllString(sanitized, "-")

	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}

// IsPortActive checks to see if the given port on Docker IP is answering.
func IsPortActive(port string) bool {
	dialTimeout := 1 * time.Second

	dockerIP, err := dockerutil.GetDockerIP()
	if err != nil {
		util.Warning("Failed to get Docker IP address: %v", err)
		return false
	}

	// Skip port check for remote Docker hosts (non-local IPs)
	// Remote IPs may cause timeouts and false positives
	if parsedIP := net.ParseIP(dockerIP); parsedIP != nil && !parsedIP.IsLoopback() {
		localIPs, _ := util.GetLocalIPs()
		if !slices.Contains(localIPs, dockerIP) {
			util.Debug("Skipping port check for remote Docker host %s:%s", dockerIP, port)
			return false
		}
	}

	util.Debug("Checking if port %s is active", port)
	conn, err := net.DialTimeout("tcp", dockerIP+":"+port, dialTimeout)

	// If we were able to connect, something is listening on the port.
	if err == nil {
		_ = conn.Close()
		return true
	}

	// In WSL2 mirrored mode, when we test an unused port, we just get a timeout
	// Assume that the port is available (not active) in that situation.
	// This seems to be caused by https://github.com/microsoft/WSL/issues/10855
	// We don't have a way to know whether WSL2 in mirrored mode, but
	// we use the longer timeout in WSL2 and assume that timeout is unoccupied.
	if util.IsWSL2() {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			util.Debug("In WSL2 and port %s is probably not active; timeout", port)
			return false
		}
	}

	// If we get ECONNREFUSED the port is not active.
	oe, ok := err.(*net.OpError)
	if ok {
		syscallErr, ok := oe.Err.(*os.SyscallError)

		// On Windows, WSAECONNREFUSED (10061) results instead of ECONNREFUSED. And golang doesn't seem to have it.
		var WSAECONNREFUSED syscall.Errno = 10061

		if ok && (syscallErr.Err == syscall.ECONNREFUSED || syscallErr.Err == WSAECONNREFUSED) {
			util.Debug("port %s shows connection refused so not active", port)
			return false
		}
	}
	// Otherwise, hmm, something else happened. It's not a fatal but can be reported.
	util.Warning("Unable to properly check port status for %s:%s: err=%v", dockerIP, port, err)
	return false
}

// ExecOpts contains options for running a command inside a container
type ExecOpts struct {
	Service   string   // Service is the service, as in 'web'
	Dir       string   // Dir is the full path to the working directory inside the container
	Cmd       string   // Cmd is the string to execute via bash/sh
	RawCmd    []string // RawCmd is the array to execute if not using
	NoCapture bool     // Nocapture if true causes use of ComposeNoCapture, so the stdout and stderr go right to stdout/stderr
	Tty       bool     // Tty if true causes a tty to be allocated
	Stdout    *os.File // Stdout can be overridden with a File
	Stderr    *os.File // Stderr can be overridden with a File
	Detach    bool     // Detach does docker-compose detach
	Env       []string // Env is the array of environment variables
	User      string   // User is the user to run as inside the container
}

// Exec executes a given command in the container of given type without allocating a pty
// Returns ComposeCmd results of stdout, stderr, err
// If Nocapture arg is true, stdout/stderr will be empty and output directly to stdout/stderr
func (p *Project) Exec(opts *ExecOpts) (string, error) {
	p.DockerEnv()

	if opts.Cmd == "" && len(opts.RawCmd) == 0 {
		return "", fmt.Errorf("no command provided")
	}

	if opts.Service == "" {
		opts.Service = "web"
	}

	state, err := dockerutil.GetContainerStateByName(fmt.Sprintf("lodev-%s-%s", p.Name, opts.Service))
	if err != nil || state != "running" {
		switch state {
		case "doesnotexist":
			return "", fmt.Errorf("service %s does not exist in project %s (state=%s)", opts.Service, p.Name, state)
		case "exited":
			return "", fmt.Errorf("service %s has exited; state=%s", opts.Service, state)
		default:
			return "", fmt.Errorf("service %s is not currently running in project %s (state=%s), use `lodev logs -s %s` to see what happened to it", opts.Service, p.Name, state, opts.Service)
		}
	}

	baseComposeExecCmd := []string{"exec"}
	if opts.Dir != "" {
		baseComposeExecCmd = append(baseComposeExecCmd, "-w", opts.Dir)
	}

	if !opts.Tty {
		baseComposeExecCmd = append(baseComposeExecCmd, "-T")
	}

	if opts.Detach {
		baseComposeExecCmd = append(baseComposeExecCmd, "--detach")
	}

	if opts.User != "" {
		baseComposeExecCmd = append(baseComposeExecCmd, "-u", opts.User)
	}

	if len(opts.Env) > 0 {
		for _, envVar := range opts.Env {
			baseComposeExecCmd = append(baseComposeExecCmd, "-e", envVar)
		}
	}

	baseComposeExecCmd = append(baseComposeExecCmd, opts.Service)

	// Cases to handle
	// - Free form, all unquoted. Like `ls -l -a`
	// - Quoted to delay pipes and other features to container, like `"ls -l -a | grep junk"`
	// Note that a set quoted on the host in lodev e will come through as a single arg

	if len(opts.RawCmd) == 0 { // Use opts.Cmd and prepend with bash
		// Use Bash for our containers, sh for 3rd-party containers
		// that may not have Bash.
		shell := "bash"
		if !slices.Contains([]string{"web"}, opts.Service) {
			shell = "sh"
		}
		errcheck := "set -eu"
		opts.RawCmd = []string{shell, "-c", errcheck + ` && ( ` + opts.Cmd + `)`}
	}

	stdout := os.Stdout
	stderr := os.Stderr
	if opts.Stdout != nil {
		stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		stderr = opts.Stderr
	}

	r := append(baseComposeExecCmd, opts.RawCmd...)
	if opts.NoCapture || opts.Tty {
		err := ComposeWithStreams(&ComposeCmdOpts{
			ComposeFiles: []string{p.DockerComposeFullRenderedYAMLPath()},
			Action:       r,
		}, os.Stdin, stdout, stderr)
		return "", err
	}
	buf, _ := ComposeCmd(&ComposeCmdOpts{
		ComposeFiles: []string{p.DockerComposeFullRenderedYAMLPath()},
		Action:       r,
	})

	return buf.String(), err
}

// Logs returns logs for a site's given container.
// See docker.LogsOptions for more information about valid tailLines values.
func (p *Project) Logs(service string, follow bool, timestamps bool, tailLines string) error {
	ctx, apiClient, err := dockerutil.GetDockerClient()
	if err != nil {
		return err
	}

	var c *container.Summary
	if service == "lodev-router" {
		c, err = dockerutil.FindContainerByLabels(map[string]string{
			"com.docker.compose.service": service,
			"com.docker.compose.oneoff":  "False",
		})
	} else {
		c, err = FindContainerByType(nodeps.WebContainer, p.GetName())
	}
	if err != nil {
		return err
	}
	if c == nil {
		util.Warning("No running service container %s was found", service)
		return nil
	}

	logOpts := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: timestamps,
	}

	if tailLines != "" {
		logOpts.Tail = tailLines
	}

	rc, err := apiClient.ContainerLogs(ctx, c.ID, logOpts)
	if err != nil {
		return err
	}
	defer rc.Close()

	// Copy logs to user output
	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, rc)
	if err != nil {
		return fmt.Errorf("failed to copy container logs: %v", err)
	}

	return nil
}

// Attach attaches to a given service container and executes a command with a TTY
func (p *Project) Attach(opts *ExecOpts) error {
	p.DockerEnv()

	if opts.Service == "" {
		opts.Service = "web"
	}

	state, err := dockerutil.GetContainerStateByName(fmt.Sprintf("lodev-%s-%s", p.Name, opts.Service))
	if err != nil || state != "running" {
		return fmt.Errorf("service %s is not running in project %s (state=%s)", opts.Service, p.Name, state)
	}

	args := []string{"exec"}
	if opts.Dir != "" {
		args = append(args, "-w", opts.Dir)
	}
	if opts.User != "" {
		args = append(args, "-u", opts.User)
	}
	args = append(args, opts.Service)
	if opts.Cmd == "" {
		return fmt.Errorf("no command provided")
	}
	args = append(args, "bash", "-c", opts.Cmd)

	return ComposeWithStreams(&ComposeCmdOpts{
		ComposeFiles: []string{p.DockerComposeFullRenderedYAMLPath()},
		Action:       args,
	}, os.Stdin, os.Stdout, os.Stderr)
}

// GetProjectContainers retrieves docker containers for a given project name.
func GetProjectContainers(projectName string) ([]container.Summary, error) {
	label := map[string]string{
		dockerutil.LabelAppName:     projectName,
		"com.docker.compose.oneoff": "False",
	}
	containers, err := dockerutil.FindContainersByLabels(label)
	if err != nil {
		return containers, err
	}
	return containers, nil
}
