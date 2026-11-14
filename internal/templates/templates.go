// Package templates renders lodev's embedded compose and webserver templates.
package templates

import (
	"bytes"
	"maps"
	"slices"
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

// Volume is a named Docker volume lodev created for the project folder Dir (relative to the root).
type Volume struct{ Name, Dir string }

type ProjectInput struct {
	Project    *config.Project
	ExtraHosts []string
	CAFile     string
	Image      string
	User       string
	UID, GID   int
	HomeDir    string // host folder mounted as /home/<User>
	Volumes    []Volume
	// Env is the project's own variables (config.yaml, then .lodev/.env); they override lodev's.
	Env map[string]string
}

// webEnv is the web service's KEY=value list: lodev's variables, then Env sorted by key.
// A $ is doubled so compose does not interpolate it.
func webEnv(in ProjectInput) []string {
	p := in.Project
	db := p.DB()
	if db == "" {
		db = "mysql"
	}
	kv := [][2]string{
		{"LODEV_PHP", p.PHP}, {"LODEV_WEBSERVER", p.Webserver}, {"LODEV_USER", in.User},
		{"LODEV_UID", strconv.Itoa(in.UID)}, {"LODEV_GID", strconv.Itoa(in.GID)}, {"LODEV_WORKDIR", p.WorkDir()},
		{"PHP_IDE_CONFIG", "serverName=" + p.IDEServerName()},
		{"DB_HOST", db}, {"DB_PORT", "3306"}, {"DB_DATABASE", p.DBName()}, {"DB_USERNAME", "root"}, {"DB_PASSWORD", "root"},
		{"REDIS_HOST", "redis"}, {"OPENSEARCH_HOST", "opensearch"},
		{"MONGODB_URI", "mongodb://root:root@mongodb:27017/?authSource=admin"},
		{"MAIL_HOST", "mailpit"}, {"MAIL_PORT", "1025"},
	}
	if p.Varnish {
		kv = append(kv, [2]string{"VARNISH_HOST", "lodev-" + p.Name + "-varnish"})
	}
	if len(p.CronLines()) > 0 {
		kv = append(kv, [2]string{"LODEV_CRON", "1"})
	}
	if len(p.WebExtraDaemons) > 0 {
		kv = append(kv, [2]string{"LODEV_DAEMONS", "1"})
	}
	if len(p.ExposedHosts()) > 0 {
		for _, e := range slices.SortedFunc(slices.Values(p.WebExtraExposedPorts), func(a, b config.ExposedPort) int { return strings.Compare(a.Name, b.Name) }) {
			kv = append(kv, [2]string{"LODEV_EXPOSED_" + strings.ToUpper(strings.ReplaceAll(e.Name, "-", "_")) + "_URL", "https://" + e.Name + "." + p.Domains[0]})
		}
	}
	if len(in.Volumes) > 0 {
		dirs := make([]string, len(in.Volumes))
		for i, v := range in.Volumes {
			dirs[i] = v.Dir
		}
		kv = append(kv, [2]string{"LODEV_VOLUME_DIRS", strings.Join(dirs, " ")})
	}
	extra := maps.Clone(in.Env)
	for i := range kv {
		if v, ok := extra[kv[i][0]]; ok {
			kv[i][1] = v
			delete(extra, kv[i][0])
		}
	}
	for _, k := range slices.Sorted(maps.Keys(extra)) {
		kv = append(kv, [2]string{k, extra[k]})
	}
	out := make([]string, len(kv))
	for i, e := range kv {
		out[i] = e[0] + "=" + strings.ReplaceAll(e[1], "$", "$$")
	}
	return out
}

func ProjectCompose(in ProjectInput) ([]byte, error) {
	return render("project-compose.yaml.tmpl", struct {
		P          *config.Project
		ExtraHosts []string
		CAFile     string
		Image      string
		User       string
		UID, GID   int
		HomeDir    string
		CustomDir  map[string]string
		Env        []string
		Cron       bool
		Volumes    []Volume
	}{in.Project, in.ExtraHosts, in.CAFile, in.Image, in.User, in.UID, in.GID, in.HomeDir, customDirs,
		webEnv(in), len(in.Project.CronLines()) > 0, in.Volumes})
}

// DaemonsConf is the supervisor program file for p's web_extra_daemons.
func DaemonsConf(p *config.Project) ([]byte, error) {
	return render("daemons.conf.tmpl", p)
}

// DaemonScript is the shell script supervisor runs for d: cd to its directory, then exec the command line.
func DaemonScript(p *config.Project, d config.Daemon) []byte {
	dir := d.Directory
	if dir == "" {
		dir = p.WorkDir()
	}
	return []byte("#!/bin/bash\ncd '" + strings.ReplaceAll(dir, "'", `'\''`) + "' || exit 1\nexec " + d.Command + "\n")
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
	root := p.WorkDir()
	if p.Docroot != "" {
		root += "/" + strings.Trim(p.Docroot, "/")
	}
	return render(name, struct{ Root, WorkDir string }{root, p.WorkDir()})
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

// VarnishVCL is p's default .lodev/varnish.vcl; its first line is GeneratedMarker.
func VarnishVCL(p *config.Project) ([]byte, error) {
	return render("varnish.vcl.tmpl", struct{ Marker, Name string }{GeneratedMarker, p.Name})
}

func TraefikDynamic(certNames []string) ([]byte, error) {
	return render("traefik-dynamic.yaml.tmpl", certNames)
}

// TraefikRoute sends HTTPS requests for Hosts to the backend at URL.
type TraefikRoute struct {
	Name  string
	Hosts []string
	URL   string
}

// Rule is Traefik's router rule matching any of Hosts.
func (r TraefikRoute) Rule() string {
	hs := make([]string, len(r.Hosts))
	for i, h := range r.Hosts {
		hs[i] = "Host(`" + h + "`)"
	}
	return strings.Join(hs, " || ")
}

// TraefikRoutes renders one dynamic-config file holding a router and a service per route.
func TraefikRoutes(rs []TraefikRoute) ([]byte, error) {
	return render("traefik-routes.yaml.tmpl", rs)
}
