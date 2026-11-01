package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
