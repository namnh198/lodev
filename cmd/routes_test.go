package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

const sharedFull = `services:
  mailpit:
    container_name: lodev-mailpit
    networks: {lodev: {}}
    environment: {VIRTUAL_HOST: mailpit.test, VIRTUAL_PORT: "8025"}
  rabbitmq:
    container_name: lodev-rabbitmq
    networks: {lodev: {}}
    environment: {VIRTUAL_HOST: rabbitmq.test, VIRTUAL_PORT: "15672"}
`

func writeSharedFull(t *testing.T) {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	os.MkdirAll(filepath.Dir(servicesFullFile()), 0o755)
	if err := os.WriteFile(servicesFullFile(), []byte(sharedFull), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteServicesRoutesOnlyRunning(t *testing.T) {
	writeSharedFull(t)
	defer func(o func(string) bool) { containerRunning = o }(containerRunning)
	containerRunning = func(c string) bool { return c == "lodev-mailpit" }
	hosts, err := sharedVirtualHosts()
	if err != nil || !slices.Equal(hosts, []string{"mailpit.test", "rabbitmq.test"}) {
		t.Fatalf("hosts %v, err %v", hosts, err)
	}
	if err := writeServicesRoutes(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(config.TraefikDir(), "lodev-services.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "s-mailpit:") || !strings.Contains(got, "http://lodev-mailpit:8025") {
		t.Errorf("mailpit route missing:\n%s", got)
	}
	if strings.Contains(got, "s-rabbitmq") {
		t.Errorf("stopped rabbitmq has a route:\n%s", got)
	}
}

func TestRemoveServicesDropsRoute(t *testing.T) {
	writeSharedFull(t)
	removed := false
	defer func(r func(...string) error, c func(string) bool) { dockerRun, containerRunning = r, c }(dockerRun, containerRunning)
	dockerRun = func(a ...string) error { removed = slices.Contains(a, "rm"); return nil }
	containerRunning = func(c string) bool { return c == "lodev-mailpit" && !removed }
	writeServicesRoutes()
	if err := removeServices([]string{"mailpit"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(config.TraefikDir(), "lodev-services.yaml")); strings.Contains(string(b), "s-mailpit") {
		t.Errorf("removed mailpit keeps its route:\n%s", b)
	}
}

func TestOldTraefik(t *testing.T) {
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)
	for _, c := range []struct {
		name, out string
		err       error
		want      bool
	}{
		{"socket", `[{"Destination":"/var/run/docker.sock"},{"Destination":"/etc/traefik/dynamic"}]`, nil, true},
		{"old certs mount", `[{"Destination":"/certs"}]`, nil, true},
		{"files only", `[{"Destination":"/etc/traefik/dynamic"}]`, nil, false},
		{"no container", "", errors.New("No such object"), false},
	} {
		dockerOutput = func(a ...string) (string, error) {
			if want := "inspect -f {{json .Mounts}} lodev-traefik"; strings.Join(a, " ") != want {
				t.Errorf("docker %v", a)
			}
			return c.out, c.err
		}
		if got := oldTraefik(); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestWarnTraefikLabels(t *testing.T) {
	_, stderr, restore := ui.Capture(false)
	defer restore()
	warnTraefikLabels([]byte("services:\n  web:\n    labels:\n      traefik.http.routers.x.rule: Host(`a`)\n"))
	want := "warning: service web: traefik labels are ignored; set VIRTUAL_HOST / VIRTUAL_PORT in the service's environment instead\n"
	if stderr.String() != want {
		t.Errorf("warning %q, want %q", stderr, want)
	}
	stderr.Reset()
	warnTraefikLabels([]byte("services:\n  web:\n    labels:\n      a.b: c\n"))
	if stderr.Len() != 0 {
		t.Errorf("unexpected warning %q", stderr)
	}
}

func TestVirtualServicesBasic(t *testing.T) {
	full := `services:
  web:
    environment: {A: b}
  adminer:
    image: adminer
    networks: {lodev: {}, default: {}}
    environment:
      VIRTUAL_HOST: " adminer.test , db.test"
      VIRTUAL_PORT: "8080"
`
	got, err := virtualServices([]byte(full), "lodev-shop", "test", []string{"web"})
	if err != nil {
		t.Fatal(err)
	}
	want := []virtualService{{"adminer", "lodev-shop-adminer-1", []string{"adminer.test", "db.test"}, 8080}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestVirtualServicesDefaults(t *testing.T) {
	full := `services:
  zed:
    networks: {lodev: {}}
    environment: {VIRTUAL_HOST: zed.test}
  adminer:
    container_name: my-adminer
    networks: {lodev: {}}
    environment: {VIRTUAL_HOST: adminer.test}
  plain:
    networks: {lodev: {}}
`
	got, err := virtualServices([]byte(full), "lodev-services", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []virtualService{
		{"adminer", "my-adminer", []string{"adminer.test"}, 80},
		{"zed", "lodev-services-zed-1", []string{"zed.test"}, 80},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestVirtualServicesErrors(t *testing.T) {
	svc := func(env, extra string) string {
		return "services:\n  adminer:\n    networks: {lodev: {}}\n" + extra + "    environment:\n" + env
	}
	host := "      VIRTUAL_HOST: adminer.test\n"
	tests := []struct {
		name, full, service, want string
		forbidden                 []string
	}{
		{"port zero", svc(host+"      VIRTUAL_PORT: \"0\"\n", ""), "adminer", "VIRTUAL_PORT", nil},
		{"port text", svc(host+"      VIRTUAL_PORT: abc\n", ""), "adminer", "VIRTUAL_PORT", nil},
		{"port big", svc(host+"      VIRTUAL_PORT: \"70000\"\n", ""), "adminer", "VIRTUAL_PORT", nil},
		{"uppercase", svc("      VIRTUAL_HOST: Adminer.test\n", ""), "adminer", "Adminer.test", nil},
		{"wrong tld", svc("      VIRTUAL_HOST: adminer.dev\n", ""), "adminer", "adminer.dev", nil},
		{"reserved", svc("      VIRTUAL_HOST: mailpit.test\n", ""), "adminer", "reserved", []string{"web"}},
		{"empty entry", svc("      VIRTUAL_HOST: a.test,,b.test\n", ""), "adminer", "empty", nil},
		{"duplicate", svc(host, "") + "  other:\n    networks: {lodev: {}}\n    environment: {VIRTUAL_HOST: adminer.test}\n", "other", "adminer.test", nil},
		{"no lodev network", "services:\n  adminer:\n    networks: {default: {}}\n    environment:\n" + host, "adminer", "add networks: [lodev, default]", nil},
		{"replicas", svc(host, "    deploy: {replicas: 2}\n"), "adminer", "replicas", nil},
		{"web", "services:\n  web:\n    networks: {lodev: {}}\n    environment:\n      VIRTUAL_HOST: web.test\n", "web", "domains: in .lodev/config.yaml", []string{"web", "varnish"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := virtualServices([]byte(tt.full), "lodev-shop", "test", tt.forbidden)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tt.service) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q must name %q and mention %q", err, tt.service, tt.want)
			}
		})
	}
}

func readRoutes(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(projectRouteFile(name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteProjectRoutesExposed(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	p := &config.Project{Name: "shop", Webserver: "nginx", Varnish: true, Domains: []string{"shop.test", "en.test"},
		WebExtraExposedPorts: []config.ExposedPort{{Name: "vite", ContainerPort: 5173}}}
	if err := writeProjectRoutes(p, nil); err != nil {
		t.Fatal(err)
	}
	got := readRoutes(t, "shop")
	mustContain(t, got, "p-shop:", "Host(`shop.test`) || Host(`en.test`)", "http://lodev-shop-varnish:80",
		"p-shop_vite:", "Host(`vite.shop.test`)", "http://lodev-shop-web:5173")
	if strings.Contains(got, "Host(`shop.test`) || Host(`en.test`) || Host(`vite.shop.test`)") {
		t.Errorf("exposed host in the main rule:\n%s", got)
	}
}

func TestWriteProjectRoutes(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	if want := filepath.Join(config.TraefikDir(), "p-shop.yaml"); projectRouteFile("shop") != want {
		t.Fatalf("projectRouteFile = %s", projectRouteFile("shop"))
	}
	p := &config.Project{Name: "shop", Webserver: "nginx", Domains: []string{"shop.test", "en.test"}}
	svcs := []virtualService{{"adminer", "lodev-shop-adminer-1", []string{"adminer.test"}, 8080}}
	if err := writeProjectRoutes(p, svcs); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readRoutes(t, "shop"), "p-shop:", "Host(`shop.test`) || Host(`en.test`)", "http://lodev-shop-web:80",
		"p-shop_svc_adminer:", "Host(`adminer.test`)", "http://lodev-shop-adminer-1:8080")

	p.Varnish = true
	if err := writeProjectRoutes(p, nil); err != nil {
		t.Fatal(err)
	}
	got := readRoutes(t, "shop")
	mustContain(t, got, "http://lodev-shop-varnish:80")
	if strings.Contains(got, "lodev-shop-web") || strings.Contains(got, "adminer") {
		t.Errorf("varnish routes:\n%s", got)
	}

	p.Varnish, p.Webserver = false, "none"
	if err := writeProjectRoutes(p, svcs); err != nil {
		t.Fatal(err)
	}
	got = readRoutes(t, "shop")
	mustContain(t, got, "p-shop_svc_adminer:")
	if strings.Contains(got, "p-shop:") {
		t.Errorf("webserver none has a web route:\n%s", got)
	}

	if err := writeProjectRoutes(p, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectRouteFile("shop")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("route file left without routes: %v", err)
	}
	if err := removeProjectRoutes("shop"); err != nil {
		t.Errorf("missing file: %v", err)
	}
}

func TestSyncProjectRoutesKeepsRenamedProjectFile(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	p, _ := newProjectConfig(t.TempDir(), "old", "php", "", "", "")
	reg := registry.Registry{}
	reg.Put(p) // registered as old, then renamed in the config without a restart
	reg.Save()
	p.Name = "new"
	config.SaveProject(p)
	writeFile(t, composeFull(p), "services:\n  web: {}\n")
	defer func(o func(string) bool) { containerRunning = o }(containerRunning)
	containerRunning = func(c string) bool { return c == "lodev-old-web" }
	if err := syncProjectRoutes(); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readRoutes(t, "new"), "p-new:")
}

func TestSyncProjectRoutesPrunes(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	reg := registry.Registry{}
	for _, name := range []string{"a", "b"} {
		p, _ := newProjectConfig(t.TempDir(), name, "php", "", "", "")
		config.SaveProject(p)
		writeFile(t, composeFull(p), "services:\n  web: {}\n")
		reg.Put(p)
	}
	reg.Save()
	defer func(o func(string) bool) { containerRunning = o }(containerRunning)
	containerRunning = func(c string) bool { return c == "lodev-a-web" }
	writeFile(t, projectRouteFile("b"), "stale")
	mine := filepath.Join(config.TraefikDir(), "mine.yaml")
	writeFile(t, mine, "mine")
	if err := syncProjectRoutes(); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readRoutes(t, "a"), "http://lodev-a-web:80")
	if _, err := os.Stat(projectRouteFile("b")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("p-b.yaml kept: %v", err)
	}
	if b, _ := os.ReadFile(mine); string(b) != "mine" {
		t.Errorf("mine.yaml = %q", b)
	}
}
