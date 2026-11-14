package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// servicesUpStubs makes servicesUp runnable without Docker, mkcert or /etc/hosts: compose config
// prints a merged file with only traefik, no shared container runs, and every port is free. runs records the
// docker commands; host records the cert and /etc/hosts calls.
func servicesUpStubs(t *testing.T) (runs, host *[]string) {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	t.Setenv("PATH", "") // nothing reaches a real docker or mkcert
	runs = stubDocker(t, "")
	_, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	host = &[]string{}
	oldPing, oldUp, oldPort, oldCfg, oldCert, oldHosts, oldWait := dockerPing, containerRunning, portCheck, composeConfig, ensureCert, applyHosts, waitServicesFn
	t.Cleanup(func() {
		dockerPing, containerRunning, portCheck, composeConfig, ensureCert, applyHosts, waitServicesFn = oldPing, oldUp, oldPort, oldCfg, oldCert, oldHosts, oldWait
	})
	dockerPing = func() error { return nil }
	containerRunning = func(string) bool { return false }
	portCheck = func(int) error { return nil }
	composeConfig = func(...string) (string, error) { return "services:\n  traefik:\n    networks: {lodev: {}}\n", nil }
	ensureCert = func(name string, d []string) error {
		*host = append(*host, "cert "+name+" "+strings.Join(d, " "))
		return nil
	}
	applyHosts = func(name string, d []string) error {
		*host = append(*host, "hosts "+name+" "+strings.Join(d, " "))
		return nil
	}
	waitServicesFn = func([]string, time.Duration) error { return nil }
	return runs, host
}

func TestServicesUpDockerDown(t *testing.T) {
	servicesUpStubs(t)
	dockerPing = func() error { return errors.New("Docker is not running") }
	if err := servicesUp(); err == nil || err.Error() != "Docker is not running" {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(servicesFile()); err == nil {
		t.Error("services file written with Docker down")
	}
}

func TestServicesUpPortInUse(t *testing.T) {
	runs, _ := servicesUpStubs(t)
	portCheck = func(p int) error {
		if p == 80 {
			return errors.New("port 80 is in use")
		}
		return nil
	}
	if err := servicesUp(); err == nil || err.Error() != "port 80 is in use" {
		t.Fatalf("err = %v", err)
	}
	if slices.ContainsFunc(*runs, func(c string) bool { return strings.Contains(c, " up ") }) {
		t.Errorf("compose up ran: %v", *runs)
	}
}

func TestServicesUp(t *testing.T) {
	runs, host := servicesUpStubs(t)
	dir := filepath.Dir(servicesFile())
	for _, old := range []string{"compose.yaml", "compose-full.yaml"} {
		writeFile(t, filepath.Join(dir, old), "x")
	}
	if err := servicesUp(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{servicesFile(), servicesFullFile(), filepath.Join(config.TraefikDir(), "lodev-services.yaml")} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("not written: %v", err)
		}
	}
	for _, old := range []string{"compose.yaml", "compose-full.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, old)); err == nil {
			t.Errorf("%s kept", old)
		}
	}
	domains := strings.Join(config.ReservedDomainsFor(config.TLD()), " ")
	if len(*host) != 2 || !strings.HasPrefix((*host)[0], "cert lodev-services ") || (*host)[1] != "hosts lodev-services"+strings.TrimPrefix((*host)[0], "cert lodev-services") {
		t.Errorf("host calls = %q", *host)
	}
	mustContain(t, (*host)[0], strings.Fields(domains)...)
	if !slices.ContainsFunc(*runs, func(c string) bool {
		return strings.HasPrefix(c, "compose --env-file "+os.DevNull+" -p lodev-services -f "+servicesFullFile()+" up -d traefik")
	}) {
		t.Errorf("no compose up: %v", *runs)
	}
}
