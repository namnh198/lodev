package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
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
