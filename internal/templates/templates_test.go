package templates

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"lodev/internal/config"
)

var update = flag.Bool("update", false, "rewrite golden files")

const home = "/home/u/.lodev"

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs from golden:\n%s", name, got)
	}
}

func defaultGlobal(t *testing.T) *config.Global {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	g, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func mysqlPortGlobal(t *testing.T) *config.Global {
	g := defaultGlobal(t)
	g.Ports.MySQL = "3306"
	g.Memory.OpenSearch = "1g"
	return g
}

func shop(webserver string) *config.Project {
	return &config.Project{
		Name: "shop", Type: "magento", PHP: "8.3", Webserver: webserver, Docroot: "pub",
		Domains:  []string{"shop.test", "en.test"},
		Services: []string{"mysql", "redis", "opensearch"},
		Root:     "/work/shop",
	}
}

func projectInput(webserver string) ProjectInput {
	return ProjectInput{
		Project:    shop(webserver),
		ExtraHosts: []string{"en.test", "shop.test", "mailpit.test", "phpmyadmin.test"},
		CAFile:     home + "/ca/rootCA.pem",
	}
}

func TestSharedComposeGolden(t *testing.T) {
	b, err := SharedCompose(defaultGlobal(t), home)
	golden(t, "services-default", must(t)(b, err))
}

func TestSharedComposeMySQLPort(t *testing.T) {
	b, err := SharedCompose(mysqlPortGlobal(t), home)
	golden(t, "services-mysqlport", must(t)(b, err))
}

func TestProjectComposeGolden(t *testing.T) {
	for _, ws := range []string{"nginx", "apache", "none"} {
		t.Run(ws, func(t *testing.T) {
			b, err := ProjectCompose(projectInput(ws))
			golden(t, "project-"+ws, must(t)(b, err))
		})
	}
}

func must(t *testing.T) func([]byte, error) []byte {
	return func(b []byte, err error) []byte {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
}

func TestWebserverConfigGolden(t *testing.T) {
	laravel := &config.Project{Name: "app", Type: "laravel", Webserver: "nginx", Docroot: "public"}
	cases := map[string]*config.Project{
		"nginx-default": laravel,
		"nginx-magento": shop("nginx"),
		"apache":        shop("apache"),
	}
	for name, p := range cases {
		b, err := WebserverConfig(p)
		golden(t, "webserver-"+name, must(t)(b, err))
	}
	if b, err := WebserverConfig(shop("none")); b != nil || err != nil {
		t.Fatalf("none: %q %v", b, err)
	}
}

func TestWebserverConfigEmptyDocroot(t *testing.T) {
	b, err := WebserverConfig(&config.Project{Type: "php", Webserver: "nginx"})
	if err != nil || !strings.Contains(string(b), "root /var/www/html;") {
		t.Fatalf("%s %v", b, err)
	}
}

func TestTraefikDynamicGolden(t *testing.T) {
	b, err := TraefikDynamic([]string{"lodev-services", "shop"})
	golden(t, "traefik-dynamic", must(t)(b, err))
}

type service struct {
	Ports       []string `yaml:"ports"`
	MemLimit    string   `yaml:"mem_limit"`
	Environment []string `yaml:"environment"`
	Labels      []string `yaml:"labels"`
	Volumes     []string `yaml:"volumes"`
	ExtraHosts  []string `yaml:"extra_hosts"`
	Networks    any      `yaml:"networks"`
}

func parse(t *testing.T, b []byte) map[string]service {
	t.Helper()
	var doc struct {
		Services map[string]service `yaml:"services"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	return doc.Services
}

func TestSharedComposeStructure(t *testing.T) {
	def := parse(t, must(t)(SharedCompose(defaultGlobal(t), home)))
	if len(def) != 6 {
		t.Fatalf("want 6 services, got %d", len(def))
	}
	if ip := def["traefik"].Networks.(map[string]any)["lodev"].(map[string]any)["ipv4_address"]; ip != "172.30.0.2" {
		t.Fatal("traefik ip", def["traefik"].Networks)
	}
	if def["mysql"].Ports != nil {
		t.Fatal("mysql exposed by default", def["mysql"].Ports)
	}
	if def["opensearch"].MemLimit != "2g" || !slices.Contains(def["opensearch"].Environment, "OPENSEARCH_JAVA_OPTS=-Xms1g -Xmx1g") {
		t.Fatalf("%+v", def["opensearch"])
	}
	mp := parse(t, must(t)(SharedCompose(mysqlPortGlobal(t), home)))
	if !slices.Equal(mp["mysql"].Ports, []string{"127.0.0.1:3306:3306"}) {
		t.Fatal(mp["mysql"].Ports)
	}
	for _, svcs := range []map[string]service{def, mp} {
		for name, s := range svcs {
			for _, p := range s.Ports {
				if !strings.HasPrefix(p, "127.0.0.1:") {
					t.Errorf("%s publishes %q on all interfaces", name, p)
				}
			}
		}
	}
}

func TestProjectComposeStructure(t *testing.T) {
	web := parse(t, must(t)(ProjectCompose(projectInput("nginx"))))["web"]
	if !slices.Contains(web.Labels, "traefik.http.routers.p-shop.rule=Host(`shop.test`) || Host(`en.test`)") {
		t.Fatal(web.Labels)
	}
	if !slices.Contains(web.ExtraHosts, "phpmyadmin.test:172.30.0.2") || !slices.Contains(web.Environment, "DB_DATABASE=shop") {
		t.Fatalf("%+v", web)
	}
	none := parse(t, must(t)(ProjectCompose(projectInput("none"))))["web"]
	if none.Labels != nil || len(none.ExtraHosts) != 4 {
		t.Fatalf("%+v", none)
	}
	for _, v := range none.Volumes {
		if strings.Contains(v, "webserver.conf") {
			t.Fatal("none mounts webserver.conf")
		}
	}
}

func TestProjectComposeImageFollowsPHPVersion(t *testing.T) {
	for ver, want := range map[string]string{"8.3": "image: lodev/php\n", "7.4": "image: lodev/php:legacy-7.4\n"} {
		in := projectInput("nginx")
		in.Project.PHP = ver
		b, err := ProjectCompose(in)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), want) {
			t.Errorf("PHP %s: compose lacks %q", ver, want)
		}
	}
}
