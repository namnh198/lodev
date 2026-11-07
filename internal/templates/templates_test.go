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
		CAFile:     home + "/rootCA/rootCA.pem",
		Image:      "lodev/shop-web",
		User:       "kuroko", UID: 501, GID: 20,
		HomeDir: home + "/data/home-shop",
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
	Image       string   `yaml:"image"`
	Ports       []string `yaml:"ports"`
	MemLimit    string   `yaml:"mem_limit"`
	Environment []string `yaml:"environment"`
	Labels      []string `yaml:"labels"`
	Volumes     []string `yaml:"volumes"`
	ExtraHosts  []string `yaml:"extra_hosts"`
	Networks    any      `yaml:"networks"`
	DependsOn   map[string]struct {
		Condition string `yaml:"condition"`
		Restart   bool   `yaml:"restart"`
	} `yaml:"depends_on"`
	Healthcheck struct {
		Test []string `yaml:"test"`
	} `yaml:"healthcheck"`
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
	if len(def) != 10 {
		t.Fatalf("want 10 services, got %d", len(def))
	}
	if m := def["mariadb"]; m.Image != "mariadb:11.4" || !slices.Contains(m.Volumes, "lodev-mariadb:/var/lib/mysql") || m.Ports != nil {
		t.Errorf("mariadb %+v", m)
	}
	if !slices.Contains(def["phpmyadmin"].Environment, "PMA_HOSTS=mysql") {
		t.Errorf("phpmyadmin %v", def["phpmyadmin"].Environment)
	}
	if r := def["rabbitmq"]; r.Image != "rabbitmq:4-management" || !slices.Contains(r.Volumes, "lodev-rabbitmq:/var/lib/rabbitmq") {
		t.Errorf("rabbitmq %+v", r)
	}
	if ip := def["traefik"].Networks.(map[string]any)["lodev"].(map[string]any)["ipv4_address"]; ip != "172.30.0.2" {
		t.Fatal("traefik ip", def["traefik"].Networks)
	}
	if def["mysql"].Ports != nil {
		t.Fatal("mysql exposed by default", def["mysql"].Ports)
	}
	if v := def["redis"].Volumes; !slices.Contains(v, "lodev-redis:/data") {
		t.Errorf("redis volumes %v", v)
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

func TestProjectComposeImage(t *testing.T) {
	b := must(t)(ProjectCompose(projectInput("nginx")))
	if !strings.Contains(string(b), "image: lodev/shop-web\n") {
		t.Errorf("compose lacks image:\n%s", b)
	}
}

func TestProjectComposeMounts(t *testing.T) {
	has := func(vols []string, want string) bool {
		return slices.ContainsFunc(vols, func(v string) bool { return strings.Contains(v, want) })
	}
	php := "/work/shop/.lodev/php:/etc/lodev/php.d:ro"
	for _, c := range []struct{ ws, conf, dir string }{
		{"nginx", "/work/shop/.lodev/nginx-webserver.conf:/etc/lodev/webserver.conf:ro", "/work/shop/.lodev/nginx:/etc/nginx/custom.d:ro"},
		{"apache", "/work/shop/.lodev/apache-webserver.conf:/etc/lodev/webserver.conf:ro", "/work/shop/.lodev/apache:/etc/apache2/custom.d:ro"},
	} {
		web := parse(t, must(t)(ProjectCompose(projectInput(c.ws))))["web"]
		for _, w := range []string{c.conf, c.dir, php} {
			if !has(web.Volumes, w) {
				t.Errorf("%s: missing mount %s in %v", c.ws, w, web.Volumes)
			}
		}
		if !slices.Equal(anyStrings(web.Networks), []string{"lodev", "default"}) {
			t.Errorf("%s networks: %v", c.ws, web.Networks)
		}
	}
	none := parse(t, must(t)(ProjectCompose(projectInput("none"))))["web"]
	if !has(none.Volumes, php) || has(none.Volumes, "webserver.conf") || has(none.Volumes, "custom.d") {
		t.Errorf("none mounts: %v", none.Volumes)
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, e := range v.([]any) {
		out = append(out, e.(string))
	}
	return out
}

func TestWebserverConfigMarkerAndInclude(t *testing.T) {
	laravel := &config.Project{Name: "app", Type: "laravel", Webserver: "nginx", Docroot: "public"}
	for name, p := range map[string]*config.Project{"nginx-default": laravel, "nginx-magento": shop("nginx"), "apache": shop("apache")} {
		s := string(must(t)(WebserverConfig(p)))
		if first, _, _ := strings.Cut(s, "\n"); first != GeneratedMarker {
			t.Errorf("%s: first line %q", name, first)
		}
		want := "include /etc/nginx/custom.d/*.conf;"
		if name == "apache" {
			want = "IncludeOptional /etc/apache2/custom.d/*.conf"
		}
		if !strings.Contains(s, want) {
			t.Errorf("%s lacks %q", name, want)
		}
	}
}

func TestWebBuildDockerfile(t *testing.T) {
	parts := []UserPart{
		{Name: ".lodev/web-images/Dockerfile", Content: []byte("RUN echo base\n")},
		{Name: ".lodev/web-images/Dockerfile.redis", Content: []byte("RUN echo redis\n")},
	}
	for _, c := range []struct {
		name  string
		edit  func(*config.Project)
		parts []UserPart
	}{
		{"webbuild-node", func(p *config.Project) { p.NodeJS = "20" }, nil},
		{"webbuild-ext", func(p *config.Project) { p.PHPExtensions = []string{"imagick"} }, nil},
		{"webbuild-parts", func(p *config.Project) {}, parts},
		{"webbuild-all", func(p *config.Project) {
			p.NodeJS, p.PHPExtensions, p.Packages = "20", []string{"imagick"}, []string{"vim"}
		}, parts},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := shop("nginx")
			c.edit(p)
			got, err := WebBuildDockerfile(p, c.parts)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, c.name, got)
		})
	}
}

func TestWebBuildDockerfileLegacyBase(t *testing.T) {
	p := shop("nginx")
	p.PHP, p.PHPExtensions = "7.4", []string{"imagick"}
	got, err := WebBuildDockerfile(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"FROM lodev/webserver:dev-legacy-7.4\n", "php7.4-imagick"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWebBuildDockerfileNoTrailingNewline(t *testing.T) {
	got, err := WebBuildDockerfile(shop("nginx"), []UserPart{
		{Name: "a", Content: []byte("RUN echo hi")}, {Name: "b", Content: []byte("RUN echo b")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "RUN echo hi\n# --- ") {
		t.Errorf("part not newline-terminated:\n%s", got)
	}
}

func TestProjectComposeUser(t *testing.T) {
	web := parse(t, must(t)(ProjectCompose(projectInput("nginx"))))["web"]
	for _, e := range []string{"LODEV_USER=kuroko", "LODEV_UID=501", "LODEV_GID=20"} {
		if !slices.Contains(web.Environment, e) {
			t.Fatalf("missing %s in %v", e, web.Environment)
		}
	}
	if !strings.Contains(string(must(t)(ProjectCompose(projectInput("none")))), "hostname: shop-web\n") {
		t.Fatal("hostname missing")
	}
}

func TestSharedComposeVersions(t *testing.T) {
	g := defaultGlobal(t)
	g.Versions["mysql"], g.Versions["redis"] = "8.4", "6"
	golden(t, "services-versions", must(t)(SharedCompose(g, home)))
}

func TestSharedComposeMariaDB(t *testing.T) {
	g := defaultGlobal(t)
	g.Services["mariadb"], g.Ports.MariaDB = true, "3307"
	b := must(t)(SharedCompose(g, home))
	golden(t, "services-mariadb", b)
	def := parse(t, b)
	if !slices.Equal(def["mariadb"].Ports, []string{"127.0.0.1:3307:3306"}) || !slices.Contains(def["phpmyadmin"].Environment, "PMA_HOSTS=mysql,mariadb") {
		t.Errorf("mariadb %v, phpmyadmin %v", def["mariadb"].Ports, def["phpmyadmin"].Environment)
	}
}

func TestProjectComposeMariaDB(t *testing.T) {
	in := projectInput("nginx")
	in.Project.Services = []string{"mariadb", "redis"}
	web := parse(t, must(t)(ProjectCompose(in)))["web"]
	if !slices.Contains(web.Environment, "DB_HOST=mariadb") {
		t.Errorf("env %v", web.Environment)
	}
}

func TestSharedComposeMongoDB(t *testing.T) {
	g := defaultGlobal(t)
	g.Ports.MongoDB = "27017"
	b := must(t)(SharedCompose(g, home))
	golden(t, "services-mongodb", b)
	def := parse(t, b)
	m := def["mongodb"]
	if m.Image != "mongo:8" || !slices.Equal(m.Ports, []string{"127.0.0.1:27017:27017"}) ||
		!slices.Equal(m.Volumes, []string{"lodev-mongodb:/data/db", "lodev-mongodb-config:/data/configdb"}) {
		t.Errorf("mongodb image %q ports %v volumes %v", m.Image, m.Ports, m.Volumes)
	}
	for _, e := range []string{"MONGO_INITDB_ROOT_USERNAME=root", "MONGO_INITDB_ROOT_PASSWORD=root"} {
		if !slices.Contains(m.Environment, e) {
			t.Errorf("mongodb env lacks %s: %v", e, m.Environment)
		}
	}
	// mongosh ships with mongo 5+, the legacy mongo shell with 4.4 and older (versions.mongodb may pin either).
	if h := m.Healthcheck.Test; len(h) != 2 || h[0] != "CMD-SHELL" || !strings.Contains(h[1], "mongosh --quiet") || !strings.Contains(h[1], "|| mongo --quiet") {
		t.Errorf("mongodb healthcheck %v", m.Healthcheck.Test)
	}
	me := def["mongo-express"]
	for _, e := range []string{"ME_CONFIG_MONGODB_URL=mongodb://root:root@mongodb:27017/", "ME_CONFIG_MONGODB_ENABLE_ADMIN=true",
		"ME_CONFIG_BASICAUTH_ENABLED=false", "ME_CONFIG_BASICAUTH=false"} {
		if !slices.Contains(me.Environment, e) {
			t.Errorf("mongo-express env lacks %s: %v", e, me.Environment)
		}
	}
	if me.DependsOn["mongodb"].Condition != "service_healthy" {
		t.Errorf("mongo-express depends_on %v", me.DependsOn)
	}
	for _, l := range []string{"traefik.http.routers.mongo-express.rule=Host(`mongo.test`)", "traefik.http.services.mongo-express.loadbalancer.server.port=8081"} {
		if !slices.Contains(me.Labels, l) {
			t.Errorf("mongo-express labels lack %s: %v", l, me.Labels)
		}
	}
	if def := parse(t, must(t)(SharedCompose(defaultGlobal(t), home))); def["mongodb"].Ports != nil || def["mongodb"].Image == "" {
		t.Errorf("default mongodb: image %q ports %v", def["mongodb"].Image, def["mongodb"].Ports)
	}
}

func TestProjectComposeMongoDBURI(t *testing.T) {
	web := parse(t, must(t)(ProjectCompose(projectInput("nginx"))))["web"]
	if !slices.Contains(web.Environment, "MONGODB_URI=mongodb://root:root@mongodb:27017/?authSource=admin") {
		t.Errorf("env %v", web.Environment)
	}
}

func varnishInput() ProjectInput {
	in := projectInput("nginx")
	in.Project.Varnish, in.Project.VarnishVersion = true, "9.1"
	return in
}

func TestProjectComposeVarnishGolden(t *testing.T) {
	golden(t, "project-varnish", must(t)(ProjectCompose(varnishInput())))
}

func TestProjectComposeVarnish(t *testing.T) {
	svcs := parse(t, must(t)(ProjectCompose(varnishInput())))
	web, v := svcs["web"], svcs["varnish"]
	// Container names: service names repeat across projects on the shared lodev network.
	if web.Labels != nil || !slices.Contains(web.Environment, "VARNISH_HOST=lodev-shop-varnish") {
		t.Errorf("web %+v", web)
	}
	if v.Image != "varnish:9.1" ||
		!slices.Contains(v.Labels, "traefik.http.routers.p-shop.rule=Host(`shop.test`) || Host(`en.test`)") ||
		!slices.Contains(v.Labels, "traefik.http.services.p-shop.loadbalancer.server.port=80") ||
		!slices.Contains(v.Volumes, "/work/shop/.lodev/varnish.vcl:/etc/varnish/default.vcl:ro") {
		t.Errorf("varnish %+v", v)
	}
	// Varnish resolves its backend once; a recreated web gets a new IP.
	// Varnish starts once web accepts connections, else its probe marks the backend sick (503s).
	if d := v.DependsOn["web"]; d.Condition != "service_healthy" || !d.Restart {
		t.Errorf("varnish depends_on %+v", v.DependsOn)
	}
	if !slices.Equal(web.Healthcheck.Test, []string{"CMD", "bash", "-c", "exec 3<>/dev/tcp/127.0.0.1/80"}) {
		t.Errorf("web healthcheck %v", web.Healthcheck.Test)
	}
	off := parse(t, must(t)(ProjectCompose(projectInput("nginx"))))
	if _, ok := off["varnish"]; ok || slices.ContainsFunc(off["web"].Environment, func(e string) bool { return strings.HasPrefix(e, "VARNISH_HOST=") }) {
		t.Errorf("varnish off still renders it: %+v", off)
	}
	if off["web"].Healthcheck.Test != nil {
		t.Errorf("web healthcheck without varnish: %v", off["web"].Healthcheck.Test)
	}
}

func TestVarnishVCLGolden(t *testing.T) {
	b := must(t)(VarnishVCL(shop("nginx")))
	golden(t, "varnish", b)
	for _, want := range []string{"vcl 4.1;", `.host = "lodev-shop-web";`,
		"varnish:vcl:generate --backend-host=lodev-shop-web --backend-port=80 --access-list=lodev-shop-web"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("VCL lacks %q", want)
		}
	}
	if !strings.HasPrefix(string(b), GeneratedMarker+"\n") {
		t.Error("VCL must start with the marker")
	}
}

func TestSharedComposeMountVolume(t *testing.T) {
	g := defaultGlobal(t)
	g.MountVolume, g.DataDir = true, "/data dir"
	g.MountPaths = map[string]string{"mysql": "/ssd/mysql"}
	b := must(t)(SharedCompose(g, home))
	svcs := parse(t, b)
	for svc, want := range map[string]string{
		"mysql": "/ssd/mysql:/var/lib/mysql", "mariadb": "/data dir/mariadb:/var/lib/mysql",
		"redis": "/data dir/redis:/data", "opensearch": "/data dir/opensearch:/usr/share/opensearch/data",
		"rabbitmq": "/data dir/rabbitmq:/var/lib/rabbitmq", "mongodb": "/data dir/mongodb:/data/db",
	} {
		v := svcs[svc].Volumes
		if !slices.Contains(v, want) {
			t.Errorf("%s volumes %v lack %q", svc, v, want)
		}
		if slices.ContainsFunc(v, func(s string) bool { return strings.HasPrefix(s, "lodev-"+svc+":") }) {
			t.Errorf("%s still mounts its named volume: %v", svc, v)
		}
	}
	if !slices.Contains(svcs["mongodb"].Volumes, "lodev-mongodb-config:/data/configdb") {
		t.Errorf("mongodb configdb %v", svcs["mongodb"].Volumes)
	}
	golden(t, "services-mountvolume", b)
}

func TestProjectComposeHome(t *testing.T) {
	web := parse(t, must(t)(ProjectCompose(projectInput("nginx"))))["web"]
	if !slices.Contains(web.Volumes, home+"/data/home-shop:/home/kuroko") {
		t.Errorf("web volumes %v", web.Volumes)
	}
}

// With dockerd's userns-remap, the web container would see the bind-mounted project and home
// as another UID's; host mode keeps container UIDs equal to the host's, as lodev assumes.
func TestProjectComposeWebUsesHostUserns(t *testing.T) {
	b, err := ProjectCompose(projectInput("nginx"))
	web, _, _ := strings.Cut(string(must(t)(b, err)), "\n  varnish:")
	if !strings.Contains(web, "\n    userns_mode: host\n") {
		t.Fatalf("web service has no userns_mode: host:\n%s", web)
	}
}
