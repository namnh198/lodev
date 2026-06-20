package lodev

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
	copy2 "github.com/otiai10/copy"

	"github.com/namnh198/lodev/pkg/dockerutil"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
)

type composeYAMLVars struct {
	Name                    string
	AppType                 string
	Webserver               string
	LodevGenerated          string
	MountType               string
	WebMount                string
	WebBuildContext         string
	WebBuildDockerfile      string
	DockerIP                string
	IsWindowsFS             bool
	IsRestartAlways         bool
	Timezone                string
	ComposerVersion         string
	Username                string
	UID                     string
	GID                     string
	WebWorkingDir           string
	WebEnvironment          []string
	Docroot                 string
	UploadDirsMap           []string
	CrontabEnabled          bool
	IsDevcontainer          bool
	DefaultContainerTimeout string
	WebExtraContainerPorts  []int
	WebExtraHTTPPorts       string
	WebExtraHTTPSPorts      string
	WebExtraExposedPorts    string
}

// DockerComposeYAMLPath returns the absolute path to where
// the base generated yaml file should exist for this project.
func (p *Project) DockerComposeYAMLPath() string {
	return p.GetConfigPath(".lodev-docker-compose-base.yaml")
}

// DockerComposeFullRenderedYAMLPath returns the absolute path to where the
// complete generated yaml file should exist for this project.
func (p *Project) DockerComposeFullRenderedYAMLPath() string {
	return p.GetConfigPath(".lodev-docker-compose-full.yaml")
}

// WriteDockerComposeYAML writes a .lodev-docker-compose-base.yaml and related to the .lodev directory.
// It then uses `docker-compose convert` to get a canonical version of the full compose file.
// It then makes a couple of fixups to the canonical version (networks and approot bind points) by
// marshaling the canonical version to YAML and then unmarshaling it back into a canonical version.
func (p *Project) WriteDockerComposeYAML() error {
	var err error

	// Create a host working_dir for the web service beforehand.
	// Otherwise, Docker will create it as root user (when Mutagen is disabled).
	// This problem (particularly for Docker volumes) is described in
	// https://github.com/moby/moby/issues/2259
	hostWorkingDir := p.GetHostWorkingDir("")
	if hostWorkingDir != "" {
		_ = os.MkdirAll(hostWorkingDir, 0755)
	}

	rendered, err := p.RenderComposeYAML()
	if err != nil {
		return err
	}

	baseYAMLPath := p.DockerComposeYAMLPath()
	baseContentBytes := []byte(rendered)
	// If the file already exists and has the same content, don't overwrite it.
	skipBaseWrite := !p.NoCache || LodevConfig.LastStartedVersion == nodeps.LodevVersion

	if !skipBaseWrite {
		if existingContent, err := os.ReadFile(baseYAMLPath); err == nil {
			if bytes.Equal(baseContentBytes, existingContent) {
				skipBaseWrite = true
			}
		}
	}

	if !skipBaseWrite {
		f, err := os.Create(baseYAMLPath)
		if err != nil {
			return err
		}
		defer util.CheckClose(f)

		_, err = f.Write(baseContentBytes)
		if err != nil {
			return err
		}
	}

	files, err := ComposeFiles(p.GetConfigPath(), baseYAMLPath, "docker-compose.*.y*ml")
	if err != nil {
		return err
	}

	envFiles, err := EnvFiles(p.GetConfigPath())
	if err != nil {
		return err
	}
	lodevEnv, err := EnvFiles(GetLodevConfigDir())
	if err != nil {
		return err
	}
	envFiles = append(envFiles, lodevEnv...)

	var action []string
	for _, envFile := range envFiles {
		action = append(action, "--env-file", envFile)
	}

	buf, err := ComposeCmd(&ComposeCmdOpts{
		ComposeFiles: files,
		Profiles:     []string{`*`},
		Action:       append(action, "config"),
	})

	if err != nil {
		return err
	}

	p.ComposeYAML, err = p.EnsureComposeYAML(buf.String())

	if err != nil {
		return err
	}

	fullContentsBytes, err := p.ComposeYAML.MarshalYAML()
	if err != nil {
		return err
	}
	fullContentsBytes = util.EscapeDollarSign(fullContentsBytes)
	fullPath := p.DockerComposeFullRenderedYAMLPath()

	// If the file already exists and has the same content, don't overwrite it.
	skipFullWrite := skipBaseWrite
	if !skipFullWrite {
		if existingContent, err := os.ReadFile(fullPath); err == nil {
			if bytes.Equal(fullContentsBytes, existingContent) {
				skipFullWrite = true
			}
		}
	}

	if !skipFullWrite {
		f, err := os.Create(fullPath)
		if err != nil {
			return err
		}
		defer func() {
			err = f.Close()
			if err != nil {
				util.Warning("Error closing %s: %v", f.Name(), err)
			}
		}()

		_, err = f.Write(fullContentsBytes)
		if err != nil {
			return err
		}
	}

	return nil
}

// EnsureComposeYAML makes minor changes to the `docker-compose config` output
// to make sure extra services are always compatible with lodev.
func (p *Project) EnsureComposeYAML(yamlStr string) (*composeTypes.Project, error) {
	project, err := EnsureComposeYAML(yamlStr)
	if err != nil {
		return project, err
	}
	envFiles, err := EnvFiles(p.GetConfigPath())
	if err != nil {
		return project, err
	}
	lodevEnvFiles, err := EnvFiles(GetLodevConfigDir())
	if err != nil {
		return project, err
	}
	envFiles = append(envFiles, lodevEnvFiles...)
	// Ensure that some important network properties are not overridden by users
	if _, ok := project.Networks[nodeps.LodevNetwork]; !ok {
		project.Networks[nodeps.LodevNetwork] = composeTypes.NetworkConfig{}
	}
	if _, ok := project.Networks["default"]; !ok {
		project.Networks["default"] = composeTypes.NetworkConfig{}
	}
	for name, network := range project.Networks {
		if nodeps.LodevNetwork == name {
			network.Name = nodeps.LodevNetwork
			network.External = true
		} else if name == "default" {
			network.Name = p.GetNetworkName()
			network.External = false
		}
		project.Networks[name] = network
	}

	labels := dockerutil.GetDockerLodevLabels(p.GetName(), map[string]string{
		dockerutil.LabelType:    dockerutil.LabelTypeProject,
		dockerutil.LabelAppRoot: p.AppRoot,
	})

	injectLabelsComposeYAML(project, labels)

	bindIP, err := dockerutil.GetDockerIP()

	if dockerutil.IsRemoteDockerHost() {
		bindIP = "0.0.0.0"
	}
	if err != nil {
		return project, err
	}

	hostDockerInternal := dockerutil.GetHostDockerInternal()

	// Ensure all services have required networks and environment variables
	for name, service := range project.Services {
		if _, ok := service.Networks[nodeps.LodevNetwork]; !ok {
			service.Networks[nodeps.LodevNetwork] = nil
		}
		if _, ok := service.Networks["default"]; !ok {
			service.Networks["default"] = nil
		}

		// Set up host.docker.internal based on LODEV's standard approach
		if hostDockerInternal.ExtraHost != "" {
			if service.ExtraHosts["host.docker.internal"] == nil {
				service.ExtraHosts["host.docker.internal"] = []string{}
			}
			if !slices.Contains(service.ExtraHosts["host.docker.internal"], hostDockerInternal.ExtraHost) {
				service.ExtraHosts["host.docker.internal"] = append(service.ExtraHosts["host.docker.internal"], hostDockerInternal.ExtraHost)
			}
		}

		service.Environment["HOST_DOCKER_INTERNAL_IP"] = &hostDockerInternal.IPAddress

		// Add environment variables from .env files to services
		for _, envFile := range envFiles {
			filename := filepath.Base(envFile)
			// Variables from .lodev/.env should be available in all containers,
			// and variables from .lodev/.env.* should only be available in a specific container.
			if filename == ".env" || filename == ".env."+name {
				envMap, _, err := ReadEnvFile(envFile)
				if err != nil && !os.IsNotExist(err) {
					util.Failed("Unable to read %s file: %v", envFile, err)
				}
				for envKey, envValue := range envMap {
					val := envValue
					service.Environment[envKey] = &val
				}
			}
		}

		// Assign the host_ip for each port if it's not already set.
		// This is needed for custom-defined user ports. For example:
		// ports:
		//   - 3000:3000
		// Without this, Docker doesn't add a Docker IP, like this:
		// ports:
		//   - 127.0.0.1:3000:3000
		for i, port := range service.Ports {
			if port.HostIP == "" {
				port.HostIP = bindIP
			}
			service.Ports[i] = port
		}

		project.Services[name] = service
	}

	return project, err
}

// RenderComposeYAML renders the contents of .lodev/.lodev-docker-compose*.yaml
func (p *Project) RenderComposeYAML() (string, error) {
	var doc bytes.Buffer
	var err error

	hostDockerInternal := dockerutil.GetHostDockerInternal()
	util.Debug("%s", hostDockerInternal.Message)
	webEnvironment := LodevConfig.WebEnvironment
	localWebEnvironment := p.WebEnvironment
	for _, v := range localWebEnvironment {
		// docker-compose won't accept a duplicate environment value
		if !slices.Contains(webEnvironment, v) {
			webEnvironment = append(webEnvironment, v)
		}
	}

	uid, gid, username := dockerutil.GetContainerUser()
	timezone := p.Timezone
	if timezone == "" {
		timezone, err = GetLocalTimezone()
		util.Debug("Using local timezone: %s", timezone)
	}

	templateVars := composeYAMLVars{
		Name:                    p.Name,
		AppType:                 p.Type,
		Webserver:               p.Webserver,
		LodevGenerated:          nodeps.LodevFileSignature,
		IsWindowsFS:             util.IsWindows(),
		MountType:               "bind",
		WebMount:                p.AppRoot,
		Timezone:                timezone,
		ComposerVersion:         p.ComposerVersion,
		Username:                username,
		IsRestartAlways:         p.RestartAlways,
		UID:                     uid,
		GID:                     gid,
		WebBuildContext:         "./.webimageBuild",
		WebWorkingDir:           p.GetAbsAppRoot(true),
		WebEnvironment:          webEnvironment,
		Docroot:                 p.Docroot,
		UploadDirsMap:           []string{},
		CrontabEnabled:          len(p.Crontab) > 0,
		IsDevcontainer:          util.IsDevcontainer(),
		DefaultContainerTimeout: fmt.Sprintf("%s", LodevConfig.ContainerWaitTimeout),
	}

	webimageExtraHTTPPorts := []string{}
	webimageExtraHTTPSPorts := []string{}
	webExtraContainerPorts := []int{}
	for _, a := range p.WebExtraExposedPorts {
		webimageExtraHTTPPorts = append(webimageExtraHTTPPorts, fmt.Sprintf("%d:%d", a.HTTPPort, a.ContainerPort))
		webimageExtraHTTPSPorts = append(webimageExtraHTTPSPorts, fmt.Sprintf("%d:%d", a.HTTPSPort, a.ContainerPort))
		webExtraContainerPorts = append(webExtraContainerPorts, a.ContainerPort)
	}
	templateVars.WebExtraContainerPorts = webExtraContainerPorts
	if len(webExtraContainerPorts) != 0 {
		templateVars.WebExtraHTTPPorts = "," + strings.Join(webimageExtraHTTPPorts, ",")
		templateVars.WebExtraHTTPSPorts = "," + strings.Join(webimageExtraHTTPSPorts, ",")

		templateVars.WebExtraExposedPorts = "expose:\n    - "
		// Odd way to join ints into a string from https://stackoverflow.com/a/37533144/215713
		templateVars.WebExtraExposedPorts = fmt.Sprint(templateVars.WebExtraExposedPorts) + strings.Trim(strings.Join(strings.Fields(fmt.Sprint(webExtraContainerPorts)), "\n    - "), "[]")
	}

	templateVars.DockerIP, err = dockerutil.GetDockerIP()
	if err != nil {
		return "", err
	}

	imageWebBuildDir := p.GetConfigPath(".webimageBuild")
	// We must start with a clean base directory
	if err = os.RemoveAll(imageWebBuildDir); err != nil {
		util.WarningMessage(fmt.Sprintf("unable to clean up directory %s, you may want to delete it manually: %v", imageWebBuildDir, err))
	}

	err = os.MkdirAll(imageWebBuildDir, 0755)

	if err != nil {
		return "", err
	}

	if err = p.WriteProjectDockerfile(imageWebBuildDir, p.GetConfigPath("web-build")); err != nil {
		return "", err
	}

	t, err := template.New("app_compose_template.yaml").Funcs(getTemplateFuncMap()).ParseFS(bundledAssets, "app_compose_template.yaml")
	if err != nil {
		return "", err
	}

	err = t.Execute(&doc, templateVars)

	return doc.String(), err
}

func (p *Project) WriteProjectDockerfile(fullpath string, userDockerfilePath string) error {
	fullpath = filepath.Join(fullpath, "Dockerfile")
	// Start with user-built dockerfile if there is one.
	err := os.MkdirAll(filepath.Dir(fullpath), 0755)
	if err != nil {
		return err
	}

	// Normal starting content is the arg and base image
	contents := `
#lodev-generated - Do not modify this file; your modifications will be overwritten.
ARG BASE_IMAGE="scratch"
FROM $BASE_IMAGE
SHELL ["/bin/bash", "-c"]
ARG TARGETPLATFORM
ARG TARGETARCH
ARG TARGETOS
ARG username
ARG uid
ARG gid
ARG LODEV_PHP_VERSION
RUN getent group tty || groupadd tty
RUN (groupadd --gid "$gid" "$username" || groupadd "$username" || true) && \
  (useradd -G tty -l -m -s "/bin/bash" --gid "$username" --comment '' --uid "$uid" "$username" || \
  useradd -G tty -l -m -s "/bin/bash" --gid "$username" --comment '' "$username" || \
  useradd -G tty -l -m -s "/bin/bash" --gid "$gid" --comment '' "$username" || \
  useradd -G tty -l -m -s "/bin/bash" --comment '' "$username")
`
	// If there are user dockerfiles, appends their contents
	if userDockerfilePath != "" {
		files, err := filepath.Glob(filepath.Join(userDockerfilePath, "Dockerfile*"))
		if err != nil {
			return err
		}

		for _, file := range files {
			// Skip example files
			if strings.HasSuffix(file, ".example") {
				continue
			}

			userContents, err := fileutil.ReadFileIntoString(file)
			if err != nil {
				return err
			}

			// Backward compatible fix, remove unnecessary BASE_IMAGE references
			re, err := regexp.Compile(`ARG BASE_IMAGE.*\n|FROM \$BASE_IMAGE.*\n`)
			if err != nil {
				return err
			}

			userContents = re.ReplaceAllString(userContents, "")
			contents = contents + "\n\n### From user Dockerfile " + file + ":\n" + userContents
		}
	}

	// Assets in the web-build directory copied to .webimageBuild so .webimageBuild can be "context"
	if userDockerfilePath != "" {
		err = copy2.Copy(userDockerfilePath, filepath.Dir(fullpath), copy2.Options{
			Skip: func(_ os.FileInfo, src, _ string) (bool, error) {
				// Do not copy file if it's not a context file
				return isNotDockerfileContextFile(userDockerfilePath, src)
			},
		})
		if err != nil {
			return err
		}
	}

	if slices.Contains(nodeps.LegacyPHPVersions, p.PHPVersion) {
		contents = contents + fmt.Sprintf(`
RUN update-alternatives --set php /usr/bin/php%s
RUN update-alternatives --install /usr/sbin/php-fpm php-fpm /usr/sbin/php-fpm%s 99 && update-alternatives --set php-fpm /usr/sbin/php-fpm%s
RUN chmod ugo+rw /var/log/php-fpm.log && chmod ugo+rwx /var/run
RUN chmod -fR ugo+w /etc/php /var/lib/php/modules
RUN phpdismod blackfire xdebug
`, p.PHPVersion, p.PHPVersion, p.PHPVersion)
	}

	extraWebContent := "\nRUN mkdir -p /home/$username && chown $username /home/$username"
	if p.NodeJSVersion != nodeps.DefaultNodeJSVersion {
		extraWebContent = extraWebContent + fmt.Sprintf(`
ENV N_PREFIX=/home/$username/.n
ENV N_INSTALL_VERSION="%s"
`, p.NodeJSVersion)
	}
	// Add supervisord config for WebExtraDaemons
	var supervisorGroup []string
	for _, appStart := range p.WebExtraDaemons {
		supervisorGroup = append(supervisorGroup, appStart.Name)
		supervisorConf := fmt.Sprintf(`
[program:%s]
group=webextradaemons
command=bash -c "%s; exit_code=$?; if [ $exit_code -ne 0 ]; then sleep 2; fi; exit $exit_code"
directory=%s
autostart=false
autorestart=true
startsecs=3 # Must stay up 3 sec, because "sleep 2" in case of fail
startretries=15
stdout_logfile=/var/tmp/logpipe
stdout_logfile_maxbytes=0
redirect_stderr=true
stopasgroup=true
`, appStart.Name, appStart.Command, appStart.Directory)
		err = os.WriteFile(p.GetConfigPath(fmt.Sprintf(".webimageBuild/%s.conf", appStart.Name)), []byte(supervisorConf), 0755)
		if err != nil {
			return fmt.Errorf("failed to write .webimageBuild/%s.conf: %v", appStart.Name, err)
		}
		extraWebContent = extraWebContent + fmt.Sprintf("\nADD %s.conf /etc/supervisor/conf.d\nRUN chmod 644 /etc/supervisor/conf.d/%s.conf", appStart.Name, appStart.Name)
	}
	if len(p.Crontab) > 0 {
		var cronContent string
		for _, cron := range p.Crontab {
			cronContent = cronContent + fmt.Sprintf("%s %s\n", cron.Schedule, cron.Command)
		}
		err = os.WriteFile(p.GetConfigPath(".webimageBuild/webcron"), []byte(cronContent), 0755)
		if err != nil {
			return fmt.Errorf("failed to write .webimageBuild/webcron: %v", err)
		}
		supervisorGroupCron := `#lodev-generated
[program:cron]
command=sudo /usr/sbin/cron -f -L7
autorestart=true
startretries=10
stdout_logfile=/proc/self/fd/2
stdout_logfile_maxbytes=0
redirect_stderr=true
`
		err = os.WriteFile(p.GetConfigPath(".webimageBuild/supervisor-cron.conf"), []byte(supervisorGroupCron), 0755)

		extraWebContent = extraWebContent + `
ADD webcron /etc/cron.d/webcron
ADD supervisor-cron.conf /etc/supervisor/conf.d
RUN chmod 644 /etc/cron.d/webcron
RUN cat /etc/cron.d/webcron | crontab -u ${username} -
`
	}

	if len(supervisorGroup) > 0 {
		err = os.WriteFile(p.GetConfigPath(".webimageBuild/webextradaemons.conf"), []byte("[group:webextradaemons]\nprograms="+strings.Join(supervisorGroup, ",")), 0755)
		if err != nil {
			return fmt.Errorf("failed to write .webimageBuild/webextradaemons.conf: %v", err)
		}
		extraWebContent = extraWebContent + "\nADD webextradaemons.conf /etc/supervisor/conf.d\nRUN chmod 644 /etc/supervisor/conf.d/webextradaemons.conf\n"
	}

	if extraWebContent != "" {
		contents = contents + extraWebContent
	}

	contents = contents + "\nRUN chmod 777 /run/php /var/run /var/log && chmod -f ugo+rwx /usr/local/bin /usr/local/bin/*"

	err = os.WriteFile(fullpath, []byte(contents), 0644)
	if err != nil {
		return err
	}
	return nil
}

// isNotDockerfileContextFile returns true if the given file is NOT a Dockerfile context file
// We consider files in the .lodev/web-build directory to be context files
func isNotDockerfileContextFile(userDockerfilePath string, file string) (bool, error) {
	// Directories are always context.
	if fileutil.IsDirectory(file) {
		return false, nil
	}
	// Get the relative path of the file from userDockerfilePath
	relPath, err := filepath.Rel(userDockerfilePath, file)
	if err != nil {
		return false, err
	}
	// If this is not a top-level file, it's a context file
	if strings.Contains(relPath, string(filepath.Separator)) {
		return false, nil
	}
	filename := filepath.Base(file)
	// Return true for not context Dockerfiles
	if strings.HasPrefix(filename, "Dockerfile") || strings.HasPrefix(filename, "pre.Dockerfile") || strings.HasPrefix(filename, "prepend.Dockerfile") {
		return true, nil
	}
	// Return true for not context README.txt if it is managed by LODEV
	if filename == "README.txt" {
		if err := fileutil.CheckSignatureOrNoFile(file, nodeps.LodevFileSignature); err == nil {
			return true, nil
		}
	}
	// Otherwise, it's a context file
	return false, nil
}
