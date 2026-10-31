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

type ProjectInput struct {
	Project    *config.Project
	Image      string
	ExtraHosts []string
	CAFile     string
}

func ProjectCompose(in ProjectInput) ([]byte, error) {
	hosts := make([]string, len(in.Project.Domains))
	for i, d := range in.Project.Domains {
		hosts[i] = "Host(`" + d + "`)"
	}
	return render("project-compose.yaml.tmpl", struct {
		P          *config.Project
		Image      string
		ExtraHosts []string
		CAFile     string
		Rule       string
	}{in.Project, in.Image, in.ExtraHosts, in.CAFile, strings.Join(hosts, " || ")})
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

func TraefikDynamic(certNames []string) ([]byte, error) {
	return render("traefik-dynamic.yaml.tmpl", certNames)
}
