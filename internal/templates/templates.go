// Package templates renders lodev's embedded compose and webserver templates.
package templates

import (
	"bytes"
	"strconv"
	"strings"
	"text/template"

	"lodev/assets"
	"lodev/internal/config"
)

var tmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"q": strconv.Quote, // a Go-quoted string is a valid YAML double-quoted scalar
}).ParseFS(assets.FS, "templates/*.tmpl"))

func render(name string, data any) ([]byte, error) {
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, name, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func SharedCompose(g *config.Global, home string) ([]byte, error) {
	heap, err := config.HeapSize(g.Memory.OpenSearch)
	if err != nil {
		return nil, err
	}
	return render("services-compose.yaml.tmpl", struct {
		G    *config.Global
		Home string
		Heap string
	}{g, home, heap})
}

// GeneratedMarker is the first line of every lodev-generated file; removing it opts the file out of regeneration.
const GeneratedMarker = "# lodev-generated: delete this line to manage this file yourself"

// customDirs is where each webserver reads user snippets inside the container.
var customDirs = map[string]string{"nginx": "/etc/nginx/custom.d", "apache": "/etc/apache2/custom.d"}

type ProjectInput struct {
	Project    *config.Project
	ExtraHosts []string
	CAFile     string
	Image      string
}

func ProjectCompose(in ProjectInput) ([]byte, error) {
	hosts := make([]string, len(in.Project.Domains))
	for i, d := range in.Project.Domains {
		hosts[i] = "Host(`" + d + "`)"
	}
	return render("project-compose.yaml.tmpl", struct {
		P          *config.Project
		ExtraHosts []string
		CAFile     string
		Image      string
		Rule       string
		CustomDir  map[string]string
	}{in.Project, in.ExtraHosts, in.CAFile, in.Image, strings.Join(hosts, " || "), customDirs})
}

// WebserverConfig returns the nginx/apache config for p, or nil for webserver none.
func WebserverConfig(p *config.Project) ([]byte, error) {
	var name string
	switch {
	case p.Webserver == "apache":
		name = "apache-vhost.conf.tmpl"
	case p.Webserver == "nginx" && p.Type == "magento":
		name = "nginx-magento.conf.tmpl"
	case p.Webserver == "nginx":
		name = "nginx-default.conf.tmpl"
	default:
		return nil, nil
	}
	root := "/var/www/html"
	if p.Docroot != "" {
		root += "/" + strings.Trim(p.Docroot, "/")
	}
	return render(name, struct{ Root string }{root})
}

// UserPart is one file from .lodev/web-images/, appended to the generated Dockerfile.
type UserPart struct {
	Name    string // path shown in the "# --- Name ---" header
	Content []byte
}

// WebBuildDockerfile renders the per-project web image Dockerfile.
func WebBuildDockerfile(p *config.Project, parts []UserPart) ([]byte, error) {
	var pkgs []string
	for _, e := range p.PHPExtensions {
		pkgs = append(pkgs, "php"+p.PHP+"-"+e)
	}
	pkgs = append(pkgs, p.Packages...)
	type part struct{ Name, Content string }
	ps := make([]part, len(parts))
	for i, u := range parts {
		c := string(u.Content)
		if !strings.HasSuffix(c, "\n") {
			c += "\n"
		}
		ps[i] = part{u.Name, c}
	}
	return render("web-build.Dockerfile.tmpl", struct {
		Base, NodeJS, Packages string
		Parts                  []part
	}{config.PHPImage(p.PHP), p.NodeJS, strings.Join(pkgs, " "), ps})
}

func TraefikDynamic(certNames []string) ([]byte, error) {
	return render("traefik-dynamic.yaml.tmpl", certNames)
}
