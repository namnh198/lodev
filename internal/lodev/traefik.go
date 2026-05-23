package lodev

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
	lodevexec "github.com/namnh198/lodev/pkg/exec"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/hostname"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
)

type TraefikService struct {
	ServiceName         string
	InternalServiceName string
	InternalServicePort string
}

type TraefikRouting struct {
	ExternalHostnames []string
	ExternalPort      string
	Service           TraefikService
	IsProject         bool
	HTTPS             bool
}

// detectTraefikRouting reviews the configured services and uses their
// VIRTUAL_HOST and HTTP(S)_EXPOSE environment variables to set up routing
// for the project
func detectTraefikRouting(project *composeTypes.Project, externalHostnames ...string) ([]TraefikRouting, []string, error) {
	var table []TraefikRouting
	if project == nil || project.Services == nil {
		return table, nil, nil
	}

	for serviceName, service := range project.Services {
		var virtualHost string
		if service.Hostname != "" {
			serviceName = service.Hostname
		}
		if virtualHostPointer, ok := service.Environment["VIRTUAL_HOST"]; ok && virtualHostPointer != nil && *virtualHostPointer != "" {
			virtualHost = *virtualHostPointer
			util.Debug("VIRTUAL_HOST=%v for %s", virtualHost, serviceName)
		}
		if virtualHost == "" {
			continue
		}
		hostnames := strings.Split(virtualHost, ",")
		if httpExposePointer, ok := service.Environment["HTTP_EXPOSE"]; ok && httpExposePointer != nil && *httpExposePointer != "" {
			httpExpose := *httpExposePointer
			util.Debug("HTTP_EXPOSE=%v for %s", httpExpose, serviceName)
			routeEntries, err := processTraefikHTTPExpose(serviceName, httpExpose, false, hostnames)
			if err != nil {
				return nil, nil, err
			}
			table = append(table, routeEntries...)
		}

		if httpsExposePointer, ok := service.Environment["HTTPS_EXPOSE"]; ok && httpsExposePointer != nil && *httpsExposePointer != "" {
			httpsExpose := *httpsExposePointer
			util.Debug("HTTPS_EXPOSE=%v for %s", httpsExpose, serviceName)
			routeEntries, err := processTraefikHTTPExpose(serviceName, httpsExpose, true, hostnames)
			if err != nil {
				return nil, nil, err
			}
			table = append(table, routeEntries...)
		}
	}

	for _, r := range table {
		if r.ExternalHostnames != nil {
			externalHostnames = append(externalHostnames, r.ExternalHostnames...)
		}
	}
	externalHostnames = util.SliceToUniqueSlice(&externalHostnames)

	return table, externalHostnames, nil
}

// processTraefikHTTPExpose creates routing table entry from VIRTUAL_HOST and HTTP(S)_EXPOSE environment variables
func processTraefikHTTPExpose(serviceName, httpExpose string, isHTTPS bool, hostnames []string) ([]TraefikRouting, error) {
	var routingTable []TraefikRouting
	portPairs := strings.SplitSeq(httpExpose, ",")
	for portPair := range portPairs {
		if portPair == "" {
			continue
		}
		ports := strings.Split(portPair, ":")
		if len(ports) == 0 || len(ports) > 2 {
			util.Warning("Skipping bad HTTP_EXPOSE port pair spec %s for service %s", portPair, serviceName)
			continue
		}
		if len(ports) == 1 && ports[0] != "" {
			ports = append(ports, ports[0])
		}
		externalHostnames := make([]string, 0)

		for _, h := range hostnames {
			if !strings.Contains(h, LodevConfig.ProjectTld) {
				h = h + "." + LodevConfig.ProjectTld
			}
			h = strings.TrimSpace(h)
			if h != "" && !slices.Contains(externalHostnames, h) {
				externalHostnames = append(externalHostnames, h)
			}
		}

		routingTable = append(routingTable, TraefikRouting{
			ExternalHostnames: externalHostnames,
			ExternalPort:      ports[0],
			Service: TraefikService{
				ServiceName:         fmt.Sprintf("%s-%s", serviceName, ports[1]),
				InternalServiceName: serviceName,
				InternalServicePort: ports[1],
			},
			HTTPS: isHTTPS,
		})
	}
	return routingTable, nil
}

// pushGlobalTraefikConfig pushes the global Traefik configuration
func pushGlobalTraefikConfig() error {
	var err error
	sourceCertDir := GetLodevConfigPath("traefik", "certs")
	sourceConfigDir := GetLodevConfigPath("traefik", "config")
	destMkcertPath := GetLodevConfigPath("traefik", "mkcert")

	if err = os.MkdirAll(sourceCertDir, 0755); err != nil {
		return fmt.Errorf("failed to create Traefik certs dir: %v", err)
	}

	if err = os.MkdirAll(sourceConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create global Traefik config dir: %v", err)
	}

	if err = os.MkdirAll(destMkcertPath, 0755); err != nil {
		return fmt.Errorf("failed to create global Traefik mkcert dir: %v", err)
	}

	caRoot := GetCAROOT()
	volumeMounts := []string{fmt.Sprintf("%s:/mnt/lodev_default", GetLodevConfigDir())}

	if caRoot == "" {
		sourceCertDir = path.Join("mnt", "lodev_default", "traefik", "certs")
	}

	c := []string{
		"--cert-file", path.Join(sourceCertDir, "default_cert.crt"),
		"--key-file", path.Join(sourceCertDir, "default_key.key"),
		"localhost", "lodev-router", "lodev-router.lodev", "lodev-router.lodev_default",
	}

	if caRoot != "" {
		util.Debug("Copying mkcert CA root from %s to %s", caRoot, destMkcertPath)
		if err := fileutil.CopyDir(caRoot, destMkcertPath, true); err != nil {
			return fmt.Errorf("failed to copy mkcert certs: %v", err)
		}

		out, err := lodevexec.RunHostCommand("mkcert", c...)
		if err != nil {
			util.Failed("failed to create global mkcert certificate, check mkcert operation: %v", out)
		}
	} else {
		util.Debug("No MKCert installed. Trying to create rootCA.pem from docker")
		if !fileutil.FileExists(filepath.Join(destMkcertPath, "rootCA.pem")) {
			_, _, rootPemErr := RunSimpleContainer(nodeps.UtilitiesImage, "mkcert-install-"+util.RandString(6), []string{"mkcert", "-install"}, []string{}, []string{"CAROOT=/mnt/lodev_default/traefik/mkcert"}, volumeMounts, "", true, false, map[string]string{}, nil, nil)
			if rootPemErr != nil {
				return rootPemErr
			}
		}
		c := []string{
			"mkcert",
			"--cert-file",
			path.Join(sourceCertDir, "default_cert.crt"),
			"--key-file", filepath.Join(sourceCertDir, "default_key.key"),
			"localhost", "lodev-router", "lodev-router.lodev", "lodev-router.lodev_default",
		}

		_, _, rootPemErr := RunSimpleContainer(nodeps.UtilitiesImage, "mkcert-global-cert-"+util.RandString(6), c, []string{}, []string{}, volumeMounts, "", true, false, map[string]string{}, nil, nil)
		if rootPemErr != nil {
			return rootPemErr
		}
	}

	traefikTemplateData := map[string]any{
		"TargetCertsPath":    "/mnt/lodev_default/traefik/certs",
		"RouterPorts":        determineRouterPorts(),
		"UseLetsEncrypt":     LodevConfig.UseLetsEncrypt,
		"LetsEncryptEmail":   LodevConfig.LetsEncryptEmail,
		"TraefikMonitorPort": LodevConfig.TraefikMonitorPort,
		"HasCAROOT":          GetCAROOT() != "",
	}

	traefikDefaultConfigPath := filepath.Join(sourceConfigDir, "default_config.yaml")
	traefikStaticConfigPath := GetLodevConfigPath("traefik", ".static_config.yaml")

	// Check to see if file can be safely overwritten (has signature, is empty, or doesn't exist)
	f, err := os.Create(traefikDefaultConfigPath)
	if err != nil {
		util.Failed("Failed to create Traefik default_config.yaml file: %v", err)
	}

	t, err := template.New("traefik_default_config_template.yaml").Funcs(getTemplateFuncMap()).ParseFS(bundledAssets, "traefik_default_config_template.yaml")
	if err != nil {
		return fmt.Errorf("could not create template from traefik_default_config_template.yaml: %v", err)
	}

	err = t.Execute(f, traefikTemplateData)
	defer f.Close()
	if err != nil {
		return fmt.Errorf("could not parse traefik_default_config_template.yaml with templatedate='%v':: %v", traefikTemplateData, err)
	}

	staticConfigTemp, err := os.CreateTemp("", "static_config-")
	if err != nil {
		return err
	}

	t, err = template.New("traefik_static_config_template.yaml").Funcs(getTemplateFuncMap()).ParseFS(bundledAssets, "traefik_static_config_template.yaml")
	if err != nil {
		return fmt.Errorf("could not create template from traefik_static_config_template.yaml: %v", err)
	}

	err = t.Execute(staticConfigTemp, traefikTemplateData)
	if err != nil {
		return fmt.Errorf("could not parse traefik_static_config_template.yaml with templatedate='%v':: %v", traefikTemplateData, err)
	}
	tmpFileName := staticConfigTemp.Name()
	err = staticConfigTemp.Close()
	if err != nil {
		return err
	}

	extraStaticConfigFiles, err := filepath.Glob(GetLodevConfigPath(".static_config.*.yaml"))
	if err != nil {
		return err
	}
	resultYaml, err := util.MergeYAML(tmpFileName, extraStaticConfigFiles...)
	if err != nil {
		return err
	}
	err = os.WriteFile(traefikStaticConfigPath, []byte(resultYaml), 0755)
	if err != nil {
		return err
	}

	return nil
}

// configurateTraefikForServices configures the dynamic configuration
// and creates cert+key in .lodev/traefik/certs
func configureTraefik(routingTable []TraefikRouting, appName string, hostnames ...string) error {
	sourceCertPath := GetLodevConfigPath("traefik", "certs")
	sourceConfigPath := GetLodevConfigPath("traefik", "config")
	var err error

	// Convert externalHostnames wildcards like `*.<anything>` to `[a-zA-Z0-9-]+.local`
	for i, v := range routingTable {
		for j, h := range v.ExternalHostnames {
			if strings.HasPrefix(h, `*.`) {
				h = `[a-zA-Z0-9-]+` + strings.TrimPrefix(h, `*`)
				routingTable[i].ExternalHostnames[j] = h
			}
			if h != "" && !slices.Contains(hostnames, h) {
				hostnames = append(hostnames, h)
			}
		}
	}

	caRoot := GetCAROOT()

	if caRoot == "" {
		sourceCertPath = path.Join("mnt", "lodev_default", "traefik", "certs")
	}

	baseName := path.Join(sourceCertPath, appName)

	c := []string{"--cert-file", baseName + ".crt", "--key-file", baseName + ".key", "127.0.0.1", "localhost", "lodev-router", "lodev-router.lodev", "lodev-router.lodev_default"}
	c = append(c, hostnames...)

	if LodevConfig.ProjectTld != nodeps.ProjectTld {
		c = append(c, "*."+LodevConfig.ProjectTld)
	}
	util.Debug("mkcert %v", c)

	// Assuming the certs don't exist, or they have #lodev-generated so can be replaced, create them
	// But not if we don't have mkcert already set up.
	if caRoot != "" {
		out, err := lodevexec.RunHostCommand("mkcert", c...)
		if err != nil {
			util.Failed("Failed to create certificates for project, check mkcert operation: %v; err=%v", out, err)
		}
	} else {
		c = append([]string{"mkcert"}, c...)
		volumeMounts := []string{fmt.Sprintf("%s:/mnt/lodev_default", GetLodevConfigDir())}
		_, _, err := RunSimpleContainer(nodeps.UtilitiesImage, "mkcert-cert-"+util.RandString(6), c, []string{}, []string{}, volumeMounts, "", true, false, map[string]string{}, nil, nil)
		if err != nil {
			return err
		}
	}

	if err = hostname.AddHostsIfNeeded(hostnames); err != nil {
		return fmt.Errorf("Failed to add hosts: %v", err)
	}

	traefikTemplateData := map[string]any{
		"AppName":         appName,
		"TargetCertsPath": "/mnt/lodev_default/traefik/certs",
		"RoutingTable":    routingTable,
		"UseLetsEncrypt":  LodevConfig.UseLetsEncrypt,
		"HasCAROOT":       GetCAROOT() != "",
		"UseHTTPS":        CanUseHTTPS(),
	}

	traefikYamlFile := filepath.Join(sourceConfigPath, appName+"_config.yaml")
	err = fileutil.CheckSignatureOrNoFile(traefikYamlFile, nodeps.LodevFileSignature)
	sigExists := (err == nil)
	if !sigExists {
		util.Debug("Not creating %s because it exists and is managed by user", traefikYamlFile)
		return nil
	}

	f, err := os.Create(traefikYamlFile)
	if err != nil {
		return fmt.Errorf("failed to create Traefik config file: %v", err)
	}
	defer f.Close()
	t, err := template.New("traefik_config_template.yaml").Funcs(getTemplateFuncMap()).ParseFS(bundledAssets, "traefik_config_template.yaml")
	if err != nil {
		return fmt.Errorf("could not create template from traefik_config_template.yaml: %v", err)
	}

	err = t.Execute(f, traefikTemplateData)
	if err != nil {
		return fmt.Errorf("could not parse traefik_config_template.yaml with templatedate='%v':: %v", traefikTemplateData, err)
	}

	return nil

}

// configurateTraefikForServices configures the dynamic configuration
// and creates cert+key in .lodev/traefik/certs
func configurateTraefikForServices(sl *ServiceList) error {
	routingTable, _, err := detectTraefikRouting(sl.ComposeYAML)
	if err != nil {
		return err
	}
	if err := configureTraefik(routingTable, "services"); err != nil {
		return err
	}
	return nil
}

// configurateTraefikForProject configures the dynamic configuration
// and creates cert+key in .lodev/traefik/certs
func configurateTraefikForProject(p *Project) error {
	hostnames := p.GetHostnames()
	routingTable, _, err := detectTraefikRouting(p.ComposeYAML, hostnames...)
	if err != nil {
		return err
	}

	if err := configureTraefik(routingTable, p.Name, hostnames...); err != nil {
		return fmt.Errorf("failed to configure Traefik for project: %v", err)
	}
	return nil
}
