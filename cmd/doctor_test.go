package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/dockertools"
	"lodev/internal/hosts"
	"lodev/internal/registry"
)

const healthyState = `0|{"Status":"running","Health":{"Status":"healthy","Log":[]}}`

type doctorEnv struct {
	pingErr     error
	compose     string // dockertools.Installed for compose ("" = missing)
	mkcert      bool
	hostsText   string
	portErr     error
	states      map[string]string // container → inspect answer
	info, df    string
	globalYAML  string
	projectGone bool
	mounts      string // lodev-traefik's mounts as JSON
}

func healthyDoctor() doctorEnv {
	return doctorEnv{
		mounts:  `[{"Destination":"/etc/traefik/dynamic"}]`,
		compose: dockertools.Compose.Version(), mkcert: true,
		info: "[name=seccomp,profile=builtin]", df: "Images\t1GB (10%)\nBuild Cache\t500MB",
	}
}

func runDoctorWith(t *testing.T, e doctorEnv) []checkResult {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	if e.globalYAML != "" {
		os.WriteFile(config.GlobalPath(), []byte(e.globalYAML), 0o644)
	}
	p, _ := newProjectConfig(t.TempDir(), "shop", "php", "", "", "")
	config.SaveProject(p)
	if e.projectGone {
		os.RemoveAll(p.Root)
	}
	registry.Registry{"shop": {Path: p.Root, Domains: p.Domains}}.Save()
	hf := filepath.Join(t.TempDir(), "hosts")
	text := e.hostsText
	if text == "" {
		text = "# lodev:shop begin\n127.0.0.1 shop.test\n# lodev:shop end\n# lodev:lodev-services begin\n127.0.0.1 " +
			strings.Join(config.ReservedDomains(), " ") + "\n# lodev:lodev-services end\n"
	}
	os.WriteFile(hf, []byte(text), 0o644)
	oh, op, oi, om, opc, oo := hosts.File, dockerPing, toolInstalled, hasMkcert, portCheck, dockerOutput
	t.Cleanup(func() {
		hosts.File, dockerPing, toolInstalled, hasMkcert, portCheck, dockerOutput = oh, op, oi, om, opc, oo
	})
	hosts.File = hf
	dockerPing = func() error { return e.pingErr }
	toolInstalled = func(tl dockertools.Tool) string {
		if tl == dockertools.Compose {
			return e.compose
		}
		return tl.Version()
	}
	hasMkcert = func() bool { return e.mkcert }
	portCheck = func(int) error { return e.portErr }
	dockerOutput = func(a ...string) (string, error) {
		switch a[0] {
		case "inspect":
			if a[2] == "{{json .Mounts}}" {
				return e.mounts, nil
			}
			if s, ok := e.states[a[len(a)-1]]; ok {
				if s == "" {
					return "", errors.New("No such container")
				}
				return s, nil
			}
			return healthyState, nil
		case "info":
			return e.info, nil
		case "system":
			return e.df, nil
		}
		return "", nil
	}
	got, _ := runDoctor()
	return got
}

func row(t *testing.T, rs []checkResult, name string) checkResult {
	t.Helper()
	for _, r := range rs {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no %q row in %+v", name, rs)
	return checkResult{}
}

func TestDoctorHealthy(t *testing.T) {
	rs := runDoctorWith(t, healthyDoctor())
	for _, r := range rs {
		if r.Level != levelOK {
			t.Errorf("%s: %+v", r.Name, r)
		}
	}
}

func TestDoctorTraefikFileRow(t *testing.T) {
	const name = "Traefik reads routes from files (no Docker socket)"
	e := healthyDoctor()
	if r := row(t, runDoctorWith(t, e), name); r.Level != levelOK {
		t.Errorf("new traefik: %+v", r)
	}
	e.mounts = `[{"Destination":"/var/run/docker.sock"}]`
	if r := row(t, runDoctorWith(t, e), name); r.Level != levelFail || r.Fix != "lodev service start" {
		t.Errorf("old traefik: %+v", r)
	}
}

func TestDoctorFindsProblems(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(*doctorEnv)
		row    string
		level  int
		detail string
	}{
		{"compose missing", func(e *doctorEnv) { e.compose = "" }, "Compose", levelFail, "not downloaded"},
		{"bad config", func(e *doctorEnv) { e.globalYAML = "ports:\n  http: 0\n" }, "Global config", levelFail, "ports.http"},
		{"port held", func(e *doctorEnv) {
			e.portErr = errors.New("port 80 is in use:\nnginx 123")
			e.states = map[string]string{"lodev-traefik": ""} // ports lodev's own traefik holds are fine
		}, "Ports", levelFail, "port 80 is in use"},
		{"no CA", func(e *doctorEnv) { e.mkcert = false }, "Local CA", levelWarn, "rootCA.pem"},
		{"hosts missing", func(e *doctorEnv) { e.hostsText = "127.0.0.1 localhost\n" }, "/etc/hosts", levelFail, "shop.test"},
		{"service unhealthy", func(e *doctorEnv) {
			e.states = map[string]string{"lodev-mysql": `0|{"Status":"running","Health":{"Status":"unhealthy","Log":[{"Output":"mysql: FAILED"}]}}`}
		}, "service mysql", levelFail, "mysql: FAILED"},
		{"service down", func(e *doctorEnv) { e.states = map[string]string{"lodev-redis": ""} }, "service redis", levelFail, "not running"},
		{"project folder gone", func(e *doctorEnv) { e.projectGone = true }, "project shop", levelWarn, "lodev remove shop"},
		{"userns", func(e *doctorEnv) { e.info = "[name=seccomp name=userns]" }, "userns-remap", levelWarn, "userns-remap"},
		{"disk", func(e *doctorEnv) { e.df = "Images\t4.5GB (60%)\nBuild Cache\t1.2GB" }, "Disk", levelWarn, "lodev clean"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := healthyDoctor()
			c.mutate(&e)
			r := row(t, runDoctorWith(t, e), c.row)
			if r.Level != c.level || !strings.Contains(r.Detail+" "+r.Fix, c.detail) {
				t.Errorf("%+v, want level %d with %q", r, c.level, c.detail)
			}
		})
	}
}

func TestDoctorDockerDown(t *testing.T) {
	e := healthyDoctor()
	e.pingErr = errors.New("Docker is not running")
	rs := runDoctorWith(t, e)
	if row(t, rs, "Docker").Level != levelFail {
		t.Error("Docker row not failed")
	}
	for _, r := range rs {
		if strings.HasPrefix(r.Name, "service ") || r.Name == "Disk" || r.Name == "userns-remap" {
			t.Errorf("Docker-dependent row %q ran with Docker down", r.Name)
		}
	}
}

func TestDoctorExitCode(t *testing.T) {
	e := healthyDoctor()
	e.mkcert = false // a warning only
	runDoctorWith(t, e)
	if _, failed := runDoctor(); failed {
		t.Error("warnings only must not fail")
	}
	dockerPing = func() error { return errors.New("down") }
	if _, failed := runDoctor(); !failed {
		t.Error("a failing check must fail")
	}
}
