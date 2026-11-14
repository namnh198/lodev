package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/hosts"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

type tldFixture struct {
	t        *testing.T
	roots    map[string]string
	hosts    string
	restarts []string
	upCalls  int
	out      bytes.Buffer
}

// newTLDFixture: a lodev home on tld test, projects name → config.yaml body, a temp hosts file,
// the running web containers and whether traefik runs; restarts and services up are recorded.
func newTLDFixture(t *testing.T, projects map[string]string, running []string, traefik bool) *tldFixture {
	t.Helper()
	f := &tldFixture{t: t, roots: map[string]string{}}
	t.Setenv("LODEV_HOME", t.TempDir())
	os.MkdirAll(config.Home(), 0o755)
	os.WriteFile(config.GlobalPath(), []byte("# keep\ntld: test\n"), 0o644)
	reg := registry.Registry{}
	for name, body := range projects {
		root := filepath.Join(t.TempDir(), name)
		os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
		os.WriteFile(config.ProjectPath(root), []byte(body), 0o644)
		p, err := config.LoadProject(root)
		if err != nil {
			t.Fatal(err)
		}
		reg[name] = registry.Entry{Path: root, Domains: p.WebDomains()}
		f.roots[name] = root
	}
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
	f.hosts = filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(f.hosts, []byte("127.0.0.1 localhost\n# lodev:lodev-services begin\n127.0.0.1 mailpit.test\n# lodev:lodev-services end\n"), 0o644)
	oh, ou, or, oc, od := hosts.File, servicesUpFn, restartFn, containerRunning, dockerOutput
	t.Cleanup(func() { hosts.File, servicesUpFn, restartFn, containerRunning, dockerOutput = oh, ou, or, oc, od })
	hosts.File = f.hosts
	servicesUpFn = func() error { f.upCalls++; return nil }
	restartFn = func(p *config.Project) error { f.restarts = append(f.restarts, p.Name); return nil }
	containerRunning = func(name string) bool { return traefik && name == "lodev-traefik" }
	dockerOutput = func(a ...string) (string, error) {
		var names []string
		for _, n := range running {
			names = append(names, "lodev-"+n+"-web")
		}
		return strings.Join(names, "\n"), nil
	}
	return f
}

func (f *tldFixture) read(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func (f *tldFixture) config(name string) string { return f.read(config.ProjectPath(f.roots[name])) }

const tldA = "# mine\nname: a\ntype: php\nphp: \"8.3\"\ndomains: [a.test, x.a.test]\n"
const tldB = "name: b\ntype: php\nphp: \"8.3\"\ndomains: [b.test]\n"

func TestRetargetTLD(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA, "b": tldB}, []string{"a"}, true)
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	if got := f.config("a"); !strings.HasPrefix(got, "# mine\n") || !strings.Contains(got, "a.local") || !strings.Contains(got, "x.a.local") || strings.Contains(got, ".test") {
		t.Errorf("a config:\n%s", got)
	}
	if got := f.config("b"); !strings.Contains(got, "b.local") || strings.Contains(got, "b.test") {
		t.Errorf("b config:\n%s", got)
	}
	reg, _ := registry.Load()
	if !slices.Equal(reg["a"].Domains, []string{"a.local", "x.a.local"}) || !slices.Equal(reg["b"].Domains, []string{"b.local"}) {
		t.Errorf("registry %+v", reg)
	}
	if got := f.read(config.GlobalPath()); !strings.Contains(got, "# keep") || !strings.Contains(got, "tld: local") {
		t.Errorf("global:\n%s", got)
	}
	h := f.read(f.hosts)
	for _, want := range []string{"127.0.0.1 a.local x.a.local", "127.0.0.1 b.local", "mailpit.local", "phpmyadmin.local"} {
		if !strings.Contains(h, want) {
			t.Errorf("hosts lacks %q:\n%s", want, h)
		}
	}
	if strings.Contains(h, ".test") || !strings.Contains(h, "127.0.0.1 localhost") {
		t.Errorf("hosts:\n%s", h)
	}
	if !slices.Equal(f.restarts, []string{"a"}) || f.upCalls != 1 {
		t.Errorf("restarts %q, services up %d", f.restarts, f.upCalls)
	}
	mustContain(t, f.out.String(), "TLD changed .test → .local: 2 project(s) updated")
}

func TestRetargetDefaultDomainProject(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"c": "name: c\ntype: php\nphp: \"8.3\"\n"}, nil, false)
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	if got := f.config("c"); strings.Contains(got, "domains") {
		t.Errorf("domains key added:\n%s", got)
	}
	if reg, _ := registry.Load(); !slices.Equal(reg["c"].Domains, []string{"c.local"}) {
		t.Errorf("registry %+v", reg["c"])
	}
}

func TestRetargetSuffixOnly(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"d": "name: d\ntype: php\nphp: \"8.3\"\ndomains: [test.shop.test]\n"}, nil, false)
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	if got := f.config("d"); !strings.Contains(got, "test.shop.local") {
		t.Errorf("config:\n%s", got)
	}
}

func TestRetargetSkipsMissing(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA, "b": tldB}, nil, false)
	os.RemoveAll(f.roots["b"])
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.config("a"), "a.local") {
		t.Error("a not moved")
	}
	mustContain(t, f.out.String(), "1 project(s) updated")
}

func TestRetargetNoop(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, []string{"a"}, true)
	before := f.read(f.hosts)
	if err := retargetTLD("test", "test", &f.out); err != nil {
		t.Fatal(err)
	}
	if f.read(f.hosts) != before || len(f.restarts) != 0 || f.upCalls != 0 || f.config("a") != tldA {
		t.Errorf("noop changed something: restarts %q up %d", f.restarts, f.upCalls)
	}
}

func TestRetargetFromNeedsTLD(t *testing.T) {
	newGlobalHome(t, "")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--from-tld=test"); err == nil || err.Error() != "--from-tld needs --tld" {
		t.Errorf("err = %v", err)
	}
}

func TestRetargetAbortsOnInvalid(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA, "b": tldB}, nil, false)
	// b edited by hand after the fixture registered it.
	os.WriteFile(config.ProjectPath(f.roots["b"]), []byte("name: b\ntype: php\nphp: \"8.3\"\ndomains: [b.test, b.example]\n"), 0o644)
	files := []string{config.ProjectPath(f.roots["a"]), config.ProjectPath(f.roots["b"]), filepath.Join(config.Home(), "projects.json"), config.GlobalPath(), f.hosts}
	before := map[string]string{}
	for _, p := range files {
		before[p] = f.read(p)
	}
	err := retargetTLD("test", "local", &f.out)
	if err == nil || err.Error() != `project b: domain "b.example" does not end in .test` {
		t.Fatalf("err = %v", err)
	}
	for _, p := range files {
		if f.read(p) != before[p] {
			t.Errorf("%s changed", p)
		}
	}
}

func TestRetargetMacLocalWarning(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	old := setupOS
	t.Cleanup(func() { setupOS = old })
	setupOS = "darwin"
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.out.String(), ".local is resolved by mDNS on macOS: lookups can be slow or skip /etc/hosts; .test avoids it") {
		t.Errorf("out:\n%s", f.out.String())
	}
}

func TestRetargetRerunAfterHostsFailure(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA, "b": tldB}, nil, false)
	good := hosts.File
	hosts.File = filepath.Join(t.TempDir(), "missing", "hosts") // Apply fails: configs and global are written first
	err := retargetTLD("test", "local", &f.out)
	if err == nil || !strings.Contains(err.Error(), "rerun: lodev config-global --tld=local --from-tld=test") {
		t.Fatalf("err = %v", err)
	}
	hosts.File = good
	if err := retargetTLD("test", "local", &f.out); err != nil { // domains already on .local stay as they are
		t.Fatalf("rerun: %v", err)
	}
	if h := f.read(f.hosts); !strings.Contains(h, "127.0.0.1 a.local x.a.local") || strings.Contains(h, ".test") {
		t.Errorf("hosts after rerun:\n%s", h)
	}
}

func TestRetargetRestartFailureContinues(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA, "b": tldB}, []string{"a", "b"}, false)
	restartFn = func(p *config.Project) error {
		f.restarts = append(f.restarts, p.Name)
		if p.Name == "a" {
			return errors.New("boom")
		}
		return nil
	}
	err := retargetTLD("test", "local", &f.out)
	if err == nil || !strings.Contains(err.Error(), "boom") || !slices.Equal(f.restarts, []string{"a", "b"}) {
		t.Fatalf("err %v, restarts %q", err, f.restarts)
	}
	mustContain(t, f.out.String(), "TLD changed .test → .local")
}

func TestRetargetDockerDown(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	dockerOutput = func(...string) (string, error) { return "", errors.New("Cannot connect to the Docker daemon") }
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatalf("err = %v", err)
	}
	mustContain(t, f.out.String(), "TLD changed .test → .local")
}

func TestRetargetFromTLDAfterHandEdit(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	os.WriteFile(config.GlobalPath(), []byte("tld: local\n"), 0o644) // edited by hand: a is now invalid
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--tld=local", "--from-tld=test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.config("a"), "a.local") {
		t.Errorf("a:\n%s", f.config("a"))
	}
}

func TestConfigGlobalTLDReadsCurrent(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--tld=localhost", "--http-port=8081"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.config("a"), "a.localhost") {
		t.Errorf("a:\n%s", f.config("a"))
	}
	if g := f.read(config.GlobalPath()); !strings.Contains(g, "tld: localhost") || !strings.Contains(g, "http: 8081") {
		t.Errorf("global:\n%s", g)
	}
}

func TestRetargetInvalidProjectAborts(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	os.WriteFile(config.ProjectPath(f.roots["a"]), []byte("name: a\ntype: php\nphp: \"7.0\"\ndomains: [a.test]\n"), 0o644)
	if err := retargetTLD("test", "local", &f.out); err == nil || !strings.Contains(err.Error(), "project a:") || !strings.Contains(err.Error(), "php") {
		t.Errorf("err = %v", err)
	}
}

func TestConfigGlobalBadTLDSavesNothing(t *testing.T) {
	newGlobalHome(t, "")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--tld=Bad", "--http-port=8081"); err == nil || !strings.Contains(err.Error(), `tld "Bad"`) {
		t.Fatalf("err = %v", err)
	}
	if got := readGlobal(t); strings.Contains(got, "8081") {
		t.Errorf("port saved before the tld error:\n%s", got)
	}
}

func TestConfigGlobalShowsTLD(t *testing.T) {
	newGlobalHome(t, "tld: local\n")
	out, _, _ := runConfig(t, newConfigGlobalCmd)
	mustContain(t, out, "tld", "local")
}

func TestConfigGlobalEmptyChangeSaysNothingChanged(t *testing.T) {
	newGlobalHome(t, "")
	out, _, err := runConfig(t, newConfigGlobalCmd, "--enable-services=")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "nothing changed")
}

func TestRetargetSkippedProjectMovesItsHosts(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA, "b": tldB}, nil, false)
	os.RemoveAll(f.roots["b"])
	_, errOut, restore := ui.Capture(false)
	err := retargetTLD("test", "local", &f.out)
	restore()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "skipped b") || strings.Contains(f.out.String(), "skipped b") {
		t.Errorf("skip note: stderr %q stdout %q", errOut, f.out.String())
	}
	if h := f.read(f.hosts); strings.Contains(h, "b.test") || !strings.Contains(h, "127.0.0.1 b.local") {
		t.Errorf("hosts:\n%s", h)
	}
	if reg, _ := registry.Load(); !slices.Equal(reg["b"].Domains, []string{"b.local"}) {
		t.Errorf("registry b %+v", reg["b"])
	}
}

func TestTLDMovesOverrideDomains(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	ov := writeOverride(t, f.roots["a"], "config.local.yaml", "# mine\ndomains: [en.a.test]\n")
	reg, _ := registry.Load() // as after a start with the override: the registry has its domains
	reg["a"] = registry.Entry{Path: f.roots["a"], Domains: []string{"en.a.test"}}
	reg.Save()
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.read(ov), "# mine", "en.a.local")
	mustContain(t, f.config("a"), "a.local, x.a.local")
	if reg, _ := registry.Load(); !slices.Equal(reg["a"].Domains, []string{"en.a.local"}) {
		t.Errorf("registry %+v", reg["a"])
	}
}

func TestRetargetServicesBlockMatchesServicesUp(t *testing.T) {
	f := newTLDFixture(t, nil, nil, false)
	writeFile(t, servicesFullFile(), mergedServices)
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	// servicesDomains' list on .local, one line: the later services up rewrites nothing.
	mustContain(t, f.read(f.hosts), "# lodev:lodev-services begin\n127.0.0.1 adminer.local mailpit.local mongo.local phpmyadmin.local rabbitmq.local\n")
}

func TestRetargetKeepsVirtualHosts(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	reg, _ := registry.Load()
	e := reg["a"]
	e.Domains = append(e.Domains, "adminer.a.test")
	reg["a"] = e
	reg.Save()
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	if reg, _ := registry.Load(); !slices.Equal(reg["a"].Domains, []string{"a.local", "x.a.local", "adminer.a.local"}) {
		t.Errorf("registry %+v", reg["a"])
	}
	mustContain(t, f.read(f.hosts), "127.0.0.1 a.local x.a.local adminer.a.local")
}

func TestTLDRetargetKeepsTrusted(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA, "b": tldB}, nil, false)
	reg, _ := registry.Load()
	reg["a"] = registry.Entry{Path: f.roots["a"], Domains: reg["a"].Domains, Trusted: map[string]string{"k": "s"}}
	reg["b"] = registry.Entry{Path: f.roots["b"], Domains: reg["b"].Domains, Trusted: map[string]string{}}
	reg.Save()
	if err := retargetTLD("test", "local", &f.out); err != nil {
		t.Fatal(err)
	}
	got, _ := registry.Load()
	if got["a"].Trusted["k"] != "s" || got["b"].Trusted == nil || len(got["b"].Trusted) != 0 {
		t.Errorf("trusted lost: a %#v, b %#v", got["a"].Trusted, got["b"].Trusted)
	}
}

func TestTLDOverrideBadDomainRefusesAll(t *testing.T) {
	f := newTLDFixture(t, map[string]string{"a": tldA}, nil, false)
	ov := writeOverride(t, f.roots["a"], "config.local.yaml", "domains: [en.example]\n")
	err := retargetTLD("test", "local", &f.out)
	if err == nil || err.Error() != `project a: domain "en.example" does not end in .test` {
		t.Fatalf("err = %v", err)
	}
	if f.config("a") != tldA || f.read(ov) != "domains: [en.example]\n" {
		t.Error("a file was written")
	}
}
