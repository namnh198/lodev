package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func writeGlobal(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	if content != "" {
		os.WriteFile(filepath.Join(home, "config.yaml"), []byte(content), 0o644)
	}
}

func TestGlobalDefaultsWhenMissing(t *testing.T) {
	writeGlobal(t, "")
	g, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if g.Ports.HTTP != 80 || g.Ports.HTTPS != 443 || g.Ports.Dashboard != 8080 || g.Ports.MySQL != "" {
		t.Fatalf("%+v", g.Ports)
	}
	for _, s := range SharedServices {
		if g.Services[s] == slices.Contains(DefaultOff, s) {
			t.Errorf("%s default = %v", s, g.Services[s])
		}
	}
	if g.Memory.OpenSearch != "2g" {
		t.Fatal(g.Memory.OpenSearch)
	}
}

func TestGlobalRabbitMQDefaultOff(t *testing.T) {
	writeGlobal(t, "")
	g, _ := LoadGlobal()
	if g.Services["rabbitmq"] || !g.Services["redis"] {
		t.Fatalf("defaults %v", g.Services)
	}
	writeGlobal(t, "services:\n  redis: false\n")
	g, err := LoadGlobal()
	if err != nil || g.Services["rabbitmq"] || g.Services["redis"] || !g.Services["mysql"] {
		t.Fatalf("partial file: %v %v", err, g.Services)
	}
}

func TestGlobalPartialFileKeepsDefaults(t *testing.T) {
	writeGlobal(t, "ports:\n  https: 8443\nservices:\n  opensearch: false\n")
	g, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if g.Ports.HTTPS != 8443 || g.Ports.HTTP != 80 || g.Services["opensearch"] || !g.Services["mysql"] {
		t.Fatalf("%+v", g)
	}
}

func TestGlobalUnquotedMySQLPort(t *testing.T) {
	writeGlobal(t, "ports:\n  mysql: 3306\n")
	g, err := LoadGlobal()
	if err != nil || g.Ports.MySQL != "3306" {
		t.Fatalf("%q %v", g.Ports.MySQL, err)
	}
}

func TestEnabledServicesOrder(t *testing.T) {
	writeGlobal(t, "services:\n  opensearch: false\n")
	g, _ := LoadGlobal()
	want := []string{"mysql", "redis", "mailpit", "phpmyadmin"}
	if !reflect.DeepEqual(g.EnabledServices(), want) {
		t.Fatal(g.EnabledServices())
	}
}

func TestHeapSize(t *testing.T) {
	ok := map[string]string{"1g": "512m", "2g": "1g", "3g": "1536m", "4g": "2g"}
	for in, want := range ok {
		if got, err := HeapSize(in); err != nil || got != want {
			t.Errorf("%s→%s %v", in, got, err)
		}
	}
	for _, in := range []string{"5g", "512m", "2G", ""} {
		if _, err := HeapSize(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestGlobalValidate(t *testing.T) {
	writeGlobal(t, "")
	g, _ := LoadGlobal()
	for in, wantErr := range map[string]bool{"abc": true, "70000": true, "3306": false, "": false} {
		g.Ports.MySQL = in
		if (g.Validate() != nil) != wantErr {
			t.Errorf("mysql %q", in)
		}
	}
	g.Ports.MySQL = ""
	g.Memory.OpenSearch = "8g"
	if g.Validate() == nil {
		t.Error("memory 8g accepted")
	}
	g.Memory.OpenSearch = "2g"
	for _, c := range []struct{ key, val, want string }{
		{"mysql", "8.x", `versions.mysql "8.x" is not a version`},
		{"mysql", "", `versions.mysql "" is not a version`},
		{"postgres", "16", `unknown service "postgres"`},
		{"mysql", "8.0.36", ""},
	} {
		writeGlobal(t, "")
		g, _ := LoadGlobal()
		g.Versions[c.key] = c.val
		err := g.Validate()
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("versions.%s=%q: %v", c.key, c.val, err)
		}
	}
}

func TestGlobalVersions(t *testing.T) {
	writeGlobal(t, "")
	g, _ := LoadGlobal()
	for svc, want := range map[string]string{"mysql": "mysql:8.0", "redis": "redis:7-alpine", "rabbitmq": "rabbitmq:4-management", "opensearch": "opensearchproject/opensearch:2"} {
		if got := g.Image(svc); got != want {
			t.Errorf("%s: %s", svc, got)
		}
	}
	writeGlobal(t, "versions:\n  mysql: \"8.4\"\n")
	g, err := LoadGlobal()
	if err != nil || g.Image("mysql") != "mysql:8.4" || g.Image("redis") != "redis:7-alpine" {
		t.Fatalf("partial versions: %v %v", err, g.Versions)
	}
}

func TestGlobalSaveLoadRoundTrip(t *testing.T) {
	writeGlobal(t, "")
	g, _ := LoadGlobal()
	g.Ports.MySQL = "3307"
	g.Services["redis"] = false
	if err := SaveGlobal(g); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGlobal()
	if err != nil || !reflect.DeepEqual(got, g) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestPMAHosts(t *testing.T) {
	writeGlobal(t, "")
	g, _ := LoadGlobal()
	for _, c := range []struct {
		mysql, mariadb bool
		want           string
	}{{true, false, "mysql"}, {true, true, "mysql,mariadb"}, {false, true, "mariadb"}, {false, false, "mysql"}} {
		g.Services["mysql"], g.Services["mariadb"] = c.mysql, c.mariadb
		if got := g.PMAHosts(); got != c.want {
			t.Errorf("%+v: %q", c, got)
		}
	}
}

func TestGlobalValidateMariaDBPort(t *testing.T) {
	writeGlobal(t, "")
	g, _ := LoadGlobal()
	g.Ports.MariaDB = "x"
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), `ports.mariadb "x" is not a valid port`) {
		t.Error(err)
	}
	g.Ports.MySQL, g.Ports.MariaDB = "3306", "3306"
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Error(err)
	}
	g.Ports.MariaDB = "3307"
	if err := g.Validate(); err != nil {
		t.Error(err)
	}
}

func TestGlobalEmptyBlocksKeepDefaults(t *testing.T) {
	writeGlobal(t, "services:\nversions:\n")
	g, err := LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if g.Image("mysql") != "mysql:8.0" || !g.Services["mysql"] || g.Services["rabbitmq"] {
		t.Fatalf("versions %v, services %v", g.Versions, g.Services)
	}
}

func TestGlobalPortRules(t *testing.T) {
	cases := []struct {
		set  func(g *Global)
		want string
	}{
		{func(g *Global) { g.Ports.HTTP = 0 }, "ports.http 0 is not a valid port"},
		{func(g *Global) { g.Ports.Dashboard = 70000 }, "ports.dashboard 70000 is not a valid port"},
		{func(g *Global) { g.Ports.HTTPS = 8080 }, "ports.https and ports.dashboard are both 8080"},
		{func(g *Global) { g.Ports.MySQL = "443" }, "ports.https and ports.mysql are both 443"},
		{func(g *Global) { g.Ports.MySQL, g.Ports.MariaDB = "3306", "3306" }, "ports.mariadb must differ from ports.mysql"},
	}
	for _, c := range cases {
		g := defaultGlobal()
		c.set(g)
		if err := g.Validate(); err == nil || err.Error() != c.want {
			t.Errorf("want %q, got %v", c.want, err)
		}
	}
	if err := defaultGlobal().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestParseGlobalEmpty(t *testing.T) {
	g, err := ParseGlobal(nil)
	if err != nil || g.Ports.HTTP != 80 || g.Versions["mysql"] != "8.0" {
		t.Fatalf("%+v %v", g, err)
	}
}

func TestGlobalUpdateCheckDefault(t *testing.T) {
	g, err := ParseGlobal(nil)
	if err != nil || !g.UpdateCheck {
		t.Errorf("default: %v, %v", g.UpdateCheck, err)
	}
	g, err = ParseGlobal([]byte("update_check: false\n"))
	if err != nil || g.UpdateCheck {
		t.Errorf("update_check: false → %v, %v", g.UpdateCheck, err)
	}
}

func TestGlobalMongoDBDefaults(t *testing.T) {
	g, err := ParseGlobal([]byte("ports:\n  http: 80\n"))
	if err != nil {
		t.Fatal(err)
	}
	if g.Services["mongodb"] || g.Services["mongo-express"] {
		t.Errorf("mongodb/mongo-express must default off: %v", g.Services)
	}
	if g.Versions["mongodb"] != "8" || g.Image("mongodb") != "mongo:8" || g.Ports.MongoDB != "" {
		t.Errorf("version %q image %q port %q", g.Versions["mongodb"], g.Image("mongodb"), g.Ports.MongoDB)
	}
	if !slices.Contains(ReservedDomains(), "mongo.test") {
		t.Error("mongo.test is not reserved")
	}
	g, _ = ParseGlobal([]byte("versions:\n  mongodb: \"7\"\n"))
	if g.Image("mongodb") != "mongo:7" {
		t.Errorf("image %q", g.Image("mongodb"))
	}
}

func TestGlobalMongoDBPort(t *testing.T) {
	if _, err := ParseGlobal([]byte("ports:\n  mongodb: \"27017\"\n")); err != nil {
		t.Errorf("27017: %v", err)
	}
	for body, want := range map[string]string{
		"ports:\n  mongodb: abc\n":                         `ports.mongodb "abc" is not a valid port`,
		"ports:\n  mysql: \"3306\"\n  mongodb: \"3306\"\n": "ports.mysql and ports.mongodb are both 3306",
		"ports:\n  mysql: \"3306\"\n  mariadb: \"3306\"\n": "ports.mariadb must differ from ports.mysql",
	} {
		if _, err := ParseGlobal([]byte(body)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", body, err, want)
		}
	}
}

func TestGlobalDockerToolKeys(t *testing.T) {
	g, err := ParseGlobal(nil)
	if err != nil || g.UseDockerComposeSystem || g.UseDockerBuildxSystem {
		t.Errorf("defaults: %v %v %v", g.UseDockerComposeSystem, g.UseDockerBuildxSystem, err)
	}
	g, _ = ParseGlobal([]byte("use_docker_compose_system: true\n"))
	if !g.UseDockerComposeSystem || g.UseDockerBuildxSystem {
		t.Errorf("compose key: %v %v", g.UseDockerComposeSystem, g.UseDockerBuildxSystem)
	}
}

func TestDataPath(t *testing.T) {
	writeGlobal(t, "")
	home, _ := os.UserHomeDir()
	for in, want := range map[string]string{
		"":           filepath.Join(Home(), "data"),
		"~/x":        filepath.Join(home, "x"),
		"/srv/lodev": "/srv/lodev",
	} {
		if got := (&Global{DataDir: in}).DataPath(); got != want {
			t.Errorf("DataPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMountPath(t *testing.T) {
	writeGlobal(t, "")
	home, _ := os.UserHomeDir()
	g := &Global{}
	if got := g.MountPath("mysql"); got != "" {
		t.Errorf("off: %q", got)
	}
	g.MountVolume = true
	if got := g.MountPath("mysql"); got != filepath.Join(Home(), "data", "mysql") {
		t.Errorf("default: %q", got)
	}
	g.MountPaths = map[string]string{"mysql": "~/db"}
	if got := g.MountPath("mysql"); got != filepath.Join(home, "db") {
		t.Errorf("override: %q", got)
	}
	if got := g.MountPath("mailpit"); got != "" {
		t.Errorf("mailpit: %q", got)
	}
}

func TestValidateDataPaths(t *testing.T) {
	for want, mutate := range map[string]func(g *Global){
		`data_dir "data" must be an absolute path or start with ~/`:                                              func(g *Global) { g.DataDir = "data" },
		`mount_paths.mysql "rel" must be an absolute path or start with ~/`:                                      func(g *Global) { g.MountPaths = map[string]string{"mysql": "rel"} },
		`mount_paths: unknown service "mailpit" (allowed: mysql, mariadb, redis, opensearch, rabbitmq, mongodb)`: func(g *Global) { g.MountPaths = map[string]string{"mailpit": "/x"} },
	} {
		g := defaultGlobal()
		mutate(g)
		if err := g.Validate(); err == nil || err.Error() != want {
			t.Errorf("err = %v, want %s", err, want)
		}
	}
}

func TestValidateSharedMountPaths(t *testing.T) {
	for name, paths := range map[string]map[string]string{
		"same":                   {"mysql": "/d/db", "mariadb": "/d/db"},
		"nested":                 {"mysql": "/d/db", "mariadb": "/d/db/maria"},
		"under data_dir default": {"mysql": "/srv/data/redis"},
	} {
		g := defaultGlobal()
		g.DataDir, g.MountPaths = "/srv/data", paths
		if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "share") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	g := defaultGlobal()
	g.MountPaths = map[string]string{"mysql": "/d/mysql", "mariadb": "/d/mariadb"}
	if err := g.Validate(); err != nil {
		t.Errorf("distinct paths: %v", err)
	}
}

func TestGlobalNodeVersion(t *testing.T) {
	g, err := ParseGlobal(nil)
	if err != nil || g.NodeVersion != "24" {
		t.Fatalf("default: %q, %v", g.NodeVersion, err)
	}
	for in, ok := range map[string]bool{"lts": true, "22": true, "18.19.0": true, "22; rm": false, "": false} {
		g.NodeVersion = in
		if (g.Validate() == nil) != ok {
			t.Errorf("node_version %q: ok=%v, err=%v", in, ok, g.Validate())
		}
	}
}

func TestGlobalContainerTimeout(t *testing.T) {
	g, err := ParseGlobal(nil)
	if err != nil || g.ContainerTimeout != "120s" || g.ContainerWait() != 120*time.Second {
		t.Fatalf("default: %q %v %v", g.ContainerTimeout, g.ContainerWait(), err)
	}
	g, err = ParseGlobal([]byte("container_timeout: 3m\n"))
	if err != nil || g.ContainerWait() != 3*time.Minute {
		t.Fatalf("3m: %v %v", g.ContainerWait(), err)
	}
	for _, bad := range []string{"0s", "-5s", "abc", "120"} {
		g.ContainerTimeout = bad
		if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "container_timeout") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestGlobalTLD(t *testing.T) {
	g, err := ParseGlobal(nil)
	if err != nil || g.TLD != "test" {
		t.Fatalf("default: %q %v", g.TLD, err)
	}
	if g, err := ParseGlobal([]byte("tld: local\n")); err != nil || g.TLD != "local" {
		t.Fatalf("local: %q %v", g.TLD, err)
	}
	for _, bad := range []string{"Local", "-x", "a.b", "", "1x", "x-"} {
		g.TLD = bad
		want := fmt.Sprintf("tld %q is not a DNS label like test or local", bad)
		if err := g.Validate(); err == nil || err.Error() != want {
			t.Errorf("%q: %v", bad, err)
		}
	}
	for _, ok := range []string{"localhost", "dev", "x"} {
		g.TLD = ok
		if err := g.Validate(); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
}

func TestTLDDomains(t *testing.T) {
	writeGlobal(t, "tld: local\n")
	if got := ReservedDomains(); !slices.Equal(got, []string{"mailpit.local", "phpmyadmin.local", "rabbitmq.local", "mongo.local"}) {
		t.Errorf("reserved %q", got)
	}
	p := &Project{Name: "shop", Type: "php", PHP: "8.3"}
	p.ApplyDefaults()
	if !slices.Equal(p.Domains, []string{"shop.local"}) {
		t.Errorf("default %q", p.Domains)
	}
	p.Domains = []string{"shop.test"}
	want := `domain "shop.test" is invalid (lowercase host name ending in .local); run lodev config-global --tld=local --from-tld=test to move every project`
	if err := p.Validate(); err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
	p.Domains = []string{"a.shop.local"}
	if err := p.Validate(); err != nil {
		t.Errorf("a.shop.local: %v", err)
	}
}
