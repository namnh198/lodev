package lodev

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/otiai10/copy"

	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
)

//go:embed all:lodev_assets
//go:embed all:webserver_config_assets
//go:embed router_compose_template.yaml
//go:embed app_compose_template.yaml
//go:embed traefik_config_template.yaml
//go:embed traefik_default_config_template.yaml
//go:embed traefik_static_config_template.yaml
var bundledAssets embed.FS

func PopulateLodevAssetsAndCommands(prName string) (err error) {
	if err = fileutil.CopyEmbedAssets(bundledAssets, "lodev_assets", GetLodevConfigDir(), nil); err != nil {
		return
	}

	// Copy the CAROOT to .lodev
	if LodevConfig.MkcertCARoot != "" {
		if err = fileutil.CopyDir(LodevConfig.MkcertCARoot, GetLodevConfigPath("traefik", "mkcert"), true); err != nil {
			util.Warning("Failed to copy MKCert CAROOT. ERR: %v", err)
		}
	}

	if prName == "" {
		return
	}

	err = CopyIntoProjectAssets(prName)
	if err != nil {
		return err
	}

	// Copy the assets to the project .lodev directory
	return nil
}

// CopyIntoProjectAssets copies the shared assets into the project .lodev directory
func CopyIntoProjectAssets(prName string) error {
	approot, err := GetProjectAppRootByName(prName)
	if err != nil {
		return err
	}

	for _, asset := range nodeps.SharedAssets {
		src := GetLodevConfigPath(asset)
		dst := path.Join(approot, LodevDir, asset)
		err = copy.Copy(src, dst, copy.Options{OnSymlink: func(string) copy.SymlinkAction { return copy.Deep }})
		if err != nil {
			return err
		}
	}
	return nil
}

//go:embed webserver_config_assets
var webserverConfigAssets embed.FS

// GenerateWebserverConfig generates the default nginx and apache config files
func (p *Project) GenerateWebserverConfig() error {
	// Prevent running as root for most cases
	// We really don't want ~/.lodev to have root ownership, breaks things.
	if os.Geteuid() == 0 {
		util.Warning("Not generating webserver config files because running with root privileges")
		return nil
	}

	var items = map[string]string{
		"nginx":                        p.GetConfigPath("nginx_full", "nginx-site.conf"),
		"apache":                       p.GetConfigPath("apache", "apache-site.conf"),
		"nginx_seconddocroot_example":  p.GetConfigPath("nginx_full", "seconddocroot.conf.example"),
		"README.nginx_full.txt":        p.GetConfigPath("nginx_full", "README.nginx_full.txt"),
		"README.apache.txt":            p.GetConfigPath("apache", "README.apache.txt"),
		"apache_seconddocroot_example": p.GetConfigPath("apache", "seconddocroot.conf.example"),
	}
	for t, configPath := range items {
		err := os.MkdirAll(filepath.Dir(configPath), 0755)
		if err != nil {
			return err
		}

		if fileutil.FileExists(configPath) {
			sigExists, err := fileutil.FGrepStringInFile(configPath, nodeps.LodevFileSignature)
			if err != nil {
				return err
			}
			// If the signature doesn't exist, they have taken over the file, so return
			if !sigExists {
				continue
			}
		}

		cfgFile := fmt.Sprintf("%s-site-%s.conf", t, p.Type)
		c, err := webserverConfigAssets.ReadFile(path.Join("webserver_config_assets", cfgFile))
		if err != nil {
			c, err = webserverConfigAssets.ReadFile(path.Join("webserver_config_assets", fmt.Sprintf("%s-site-php.conf", t)))
			if err != nil {
				return err
			}
		}
		content := string(c)
		docroot := p.GetAbsDocroot(true)
		err = fileutil.TemplateStringToFile(content, map[string]any{"Docroot": docroot, "Port": p.HostWebserverPort}, configPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func IsBundledCustomCommand(service, command string) bool {
	var baseDir string
	baseDir = "lodev_assets"

	_, err := bundledAssets.ReadFile(filepath.Join(baseDir, "commands", service, command))

	return err == nil
}
