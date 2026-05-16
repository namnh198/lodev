package lodev

import (
	"os"
	"path/filepath"

	"github.com/namnh198/lodev/pkg/fileutil"
)

// List project types supported by LODEV
const (
	ProjectTypePHP       = "php"
	ProjectTypeMagento   = "magento"
	ProjectTypeOpenMage  = "openmage"
	ProjectTypeLaravel   = "laravel"
	ProjectTypeSymfony   = "symfony"
	ProjectTypeDrupal    = "drupal"
	ProjectTypeWordpress = "wordpress"
	ProjectTypePython    = "python"
	ProjectTypeDjango    = "django"
	ProjectTypeFastAPI   = "fastapi"
	ProjectTypeNodeJS    = "nodejs"
)

type ProjectType struct {
	Name                 string
	Label                string
	Docroot              string
	UploadDirs           []string
	detectFunc           func(p *Project) bool
	configOverrideAction func(p *Project) error
}

var ProjectTypeMatrix map[string]ProjectType

func init() {
	ProjectTypeMatrix = map[string]ProjectType{
		ProjectTypePHP: {
			Name:       ProjectTypePHP,
			Label:      "Generic PHP",
			Docroot:    "",
			UploadDirs: []string{},
		},
		ProjectTypeMagento: {
			Name:       ProjectTypeMagento,
			Label:      "Magento 2",
			Docroot:    "pub",
			UploadDirs: []string{"media"},
			detectFunc: func(p *Project) bool {
				ism2, err := fileutil.FGrepStringInFile(filepath.Join(p.GetAbsDocroot(false), "..", "COPYING.txt"), `license@magentocommerce.com`)
				if err == nil && ism2 {
					return true
				}
				return false
			},
		},
		ProjectTypeOpenMage: {
			Name:       ProjectTypeOpenMage,
			Label:      "OpenMage",
			Docroot:    "pub",
			UploadDirs: []string{"media"},
			detectFunc: func(p *Project) bool {
				ism1, err := fileutil.FGrepStringInFile(filepath.Join(p.GetAbsDocroot(false), "..", "COPYING.txt"), `https://openmage.org`)
				if err == nil && ism1 {
					return true
				}
				return false
			},
		},
		ProjectTypeNodeJS: {
			Name:       ProjectTypeNodeJS,
			Label:      "Node.js",
			Docroot:    "",
			UploadDirs: []string{},
			detectFunc: func(p *Project) bool {
				return fileutil.FileExists(filepath.Join(p.AppRoot, p.ComposerRoot, "package.json")) && !fileutil.FileExists(filepath.Join(p.AppRoot, p.ComposerRoot, "composer.json"))
			},
			configOverrideAction: func(p *Project) error {
				if p.HostWebserverPort == "" {
					p.HostWebserverPort = "3000"
				}
				return nil
			},
		},
		ProjectTypeLaravel: {
			Name:       ProjectTypeLaravel,
			Label:      "Laravel",
			Docroot:    "public",
			UploadDirs: []string{"storage"},
			detectFunc: func(p *Project) bool {
				return fileutil.FileExists(filepath.Join(p.AppRoot, p.ComposerRoot, "artisan"))
			},
		},
		ProjectTypeSymfony: {
			Name:       ProjectTypeSymfony,
			Label:      "Symfony",
			Docroot:    "public",
			UploadDirs: []string{"var"},
			detectFunc: func(p *Project) bool {
				return fileutil.FileExists(filepath.Join(p.AppRoot, p.ComposerRoot, "bin", "console")) && fileutil.FileExists(filepath.Join(p.AppRoot, p.ComposerRoot, "src", "Kernel.php"))
			},
		},
		ProjectTypeDrupal: {
			Name:       ProjectTypeDrupal,
			Label:      "Drupal",
			Docroot:    "web",
			UploadDirs: []string{"sites/default/files"},
			detectFunc: func(p *Project) bool {
				return fileutil.FileExists(filepath.Join(p.AppRoot, p.ComposerRoot, "core", "lib", "Drupal.php"))
			},
		},
		ProjectTypeWordpress: {
			Name:       ProjectTypeWordpress,
			Label:      "WordPress",
			Docroot:    "",
			UploadDirs: []string{"wp-content/uploads"},
			detectFunc: func(p *Project) bool {
				return fileutil.FileExists(filepath.Join(p.AppRoot, p.ComposerRoot, "wp-includes", "version.php"))
			},
		},
		ProjectTypeDjango: {
			Name:       ProjectTypeDjango,
			Label:      "Django",
			Docroot:    "",
			UploadDirs: []string{},
		},
		ProjectTypeFastAPI: {
			Name:       ProjectTypeFastAPI,
			Label:      "FastAPI",
			Docroot:    "",
			UploadDirs: []string{},
		},
	}
}

// ConfigFileOverrideAction gives a chance for an apptype to override any element
// of config.yaml that it needs to
func (p *Project) ConfigFileOverrideAction(overrideExistingConfig bool) error {
	prType := ProjectTypeMatrix[p.Type]
	if prType.configOverrideAction != nil {
		if err := prType.configOverrideAction(p); err != nil {
			return err
		}
	}
	return nil
}

// DetectProjectType calls each project type's detector until it finds a match,
// or use project type'php' if no match is found.
func (p *Project) DetectProjectType() *Project {
	for _, projectType := range ProjectTypeMatrix {
		if projectType.detectFunc != nil && projectType.detectFunc(p) {
			p.Type = projectType.Name
			return p
		}
	}

	p.Type = ProjectTypePHP
	return p
}

// DetectDocroot detects the docroot of the project if it is not set
func (p *Project) DetectDocroot() {
	if p.Docroot != "" {
		return
	}

	prType := ProjectTypeMatrix[p.Type]
	if prType.Docroot != "" {
		p.Docroot = prType.Docroot
		return
	}

	var defaultDocroot = p.Docroot

	for _, docroot := range AvailablePHPDocrootLocations() {
		if _, err := os.Stat(filepath.Join(p.AppRoot, docroot)); err != nil {
			continue
		}

		if fileutil.FileExists(filepath.Join(p.AppRoot, docroot, "index.php")) {
			defaultDocroot = docroot
			break
		}
	}

	p.Docroot = defaultDocroot
}

// GetProjectTypes returns a list of all project types' names (e.g. ["php", "magento", "laravel", etc.])
func GetProjectTypes() []string {
	projectTypes := []string{}
	for _, projectType := range ProjectTypeMatrix {
		projectTypes = append(projectTypes, projectType.Name)
	}

	return projectTypes
}
