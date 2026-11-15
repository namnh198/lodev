package templates

import (
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
)

func nextjsProject() *config.Project {
	return &config.Project{Name: "web", Type: "nextjs", PHP: "8.4", Webserver: "nginx", Root: "/srv/web", Domains: []string{"web.test"}}
}

func TestWebserverConfigNextjsGolden(t *testing.T) {
	b := must(t)(WebserverConfig(nextjsProject()))
	golden(t, "webserver-nextjs", b)
	s := string(b)
	// proxy_buffering off: streamed RSC/Suspense must reach the browser as it renders.
	for _, want := range []string{"proxy_pass http://127.0.0.1:3000;", "proxy_set_header Upgrade $http_upgrade;", "# lodev-generated",
		"proxy_buffering off;"} {
		if !strings.Contains(s, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if strings.Contains(s, "fastcgi_pass") {
		t.Error("nextjs conf talks to php-fpm")
	}
}

func TestProjectComposeNextjsGolden(t *testing.T) {
	in := projectInput("nginx")
	in.Project = nextjsProject()
	b := must(t)(ProjectCompose(in))
	golden(t, "project-nextjs", b)
	web := parse(t, b)["web"]
	for _, want := range []string{"NODE_EXTRA_CA_CERTS=/etc/lodev/rootCA.pem", "LODEV_DAEMONS=1"} {
		if !slices.Contains(web.Environment, want) {
			t.Errorf("env lacks %s: %v", want, web.Environment)
		}
	}
	if !slices.ContainsFunc(web.Volumes, func(v string) bool { return strings.HasSuffix(v, "/.lodev/daemons:/etc/lodev/daemons:ro") }) {
		t.Errorf("no daemons mount: %v", web.Volumes)
	}
	in.Env = map[string]string{"NODE_EXTRA_CA_CERTS": "/x.pem"}
	env := parse(t, must(t)(ProjectCompose(in)))["web"].Environment
	n := 0
	for _, e := range env {
		if strings.HasPrefix(e, "NODE_EXTRA_CA_CERTS=") {
			n++
			if e != "NODE_EXTRA_CA_CERTS=/x.pem" {
				t.Errorf("override lost: %s", e)
			}
		}
	}
	if n != 1 {
		t.Errorf("NODE_EXTRA_CA_CERTS listed %d times", n)
	}
	if b := string(must(t)(ProjectCompose(projectInput("nginx")))); strings.Contains(b, "NODE_EXTRA_CA_CERTS") {
		t.Error("a php project gets NODE_EXTRA_CA_CERTS")
	}
}

func TestDaemonsConfNextjs(t *testing.T) {
	if s := string(must(t)(DaemonsConf(nextjsProject()))); !strings.Contains(s, "[program:daemon-nextjs]") {
		t.Errorf("daemons.conf:\n%s", s)
	}
}
