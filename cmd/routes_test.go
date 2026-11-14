package cmd

import (
	"reflect"
	"strings"
	"testing"
)

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
