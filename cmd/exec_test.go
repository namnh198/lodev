package cmd

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/ui"
)

func TestCheckWebUser(t *testing.T) {
	old := userReady
	t.Cleanup(func() { userReady = old })
	p := &config.Project{Name: "x"}

	userReady = func(*config.Project, string) bool { return false }
	out, _, restore := ui.Capture(false)
	err := checkWebUser(p, "alice")
	restore()
	if err == nil || !strings.Contains(out.String(), "hint: this container predates the current lodev; run lodev restart") {
		t.Errorf("missing user: err=%v out=%q", err, out.String())
	}

	userReady = func(*config.Project, string) bool { t.Error("probed root"); return false }
	if err := checkWebUser(p, "root"); err != nil {
		t.Errorf("root: %v", err)
	}

	userReady = func(*config.Project, string) bool { return true }
	if err := checkWebUser(p, "alice"); err != nil {
		t.Errorf("ready user: %v", err)
	}
}

func TestShortcutCmd(t *testing.T) {
	cases := map[string]string{
		"php": "php", "composer": "composer", "npm": "npm", "node": "node",
		"artisan": "php artisan", "wp": "wp", "magento": "php bin/magento", "n98": "n98-magerun2",
	}
	for name, want := range cases {
		if got := shortcutCmd(name); !slices.Equal(got, strings.Fields(want)) {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestXdebugScript(t *testing.T) {
	on, off := xdebugScript("on"), xdebugScript("off")
	if on == "" || off == "" || xdebugScript("bogus") != "" {
		t.Fatal("on/off must exist, unknown must be empty")
	}
	want := "if php -m | grep -qi xdebug; then " + off + "; else " + on + "; fi"
	if got := xdebugScript("toggle"); got != want {
		t.Errorf("toggle = %q", got)
	}
}

func TestSplitService(t *testing.T) {
	cases := []struct {
		in      []string
		svc     string
		rest    []string
		wantErr bool
	}{
		{[]string{"ls"}, "web", []string{"ls"}, false},
		{[]string{"-s", "db", "mysql"}, "db", []string{"mysql"}, false},
		{[]string{"--service", "db", "x"}, "db", []string{"x"}, false},
		{[]string{"--service=db", "x"}, "db", []string{"x"}, false},
		{[]string{"ls", "-s"}, "web", []string{"ls", "-s"}, false},
		{[]string{"-s"}, "", nil, true},
		{[]string{"-s", ""}, "", nil, true},
		{[]string{"--service="}, "", nil, true},
		{[]string{"-s", "db"}, "db", []string{}, false},
	}
	for _, c := range cases {
		svc, rest, err := splitService(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("%v: err %v", c.in, err)
			continue
		}
		if !c.wantErr && (svc != c.svc || !reflect.DeepEqual(rest, c.rest)) {
			t.Errorf("%v: got %q %v", c.in, svc, rest)
		}
	}
}

func TestShowLogs(t *testing.T) {
	old := dockerRun
	t.Cleanup(func() { dockerRun = old })
	var got []string
	dockerRun = func(a ...string) error { got = a; return nil }

	// shared services (and traefik) are not in the project's compose: read their container
	for _, svc := range []string{"mysql", "traefik"} {
		if err := showLogs(svc); err != nil || !slices.Equal(got, []string{"logs", "-f", "lodev-" + svc}) {
			t.Fatalf("%s: %v %v", svc, got, err)
		}
	}

	p, _ := newProjectConfig(t.TempDir(), "shop", "php", "", "", "")
	config.SaveProject(p)
	t.Chdir(p.Root)
	if err := showLogs("web"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"compose", "--env-file", "/dev/null", "-p", "lodev-shop", "-f", composeFile(p), "logs", "-f", "web"}; !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
	if err := showLogs(""); err != errServiceName {
		t.Fatal(err)
	}
}

func TestShowLogsJSON(t *testing.T) {
	old := dockerPipe
	t.Cleanup(func() { dockerPipe = old })
	var from, to []string
	dockerPipe = func(a, b []string) error { from, to = a, b; return nil }
	p, _ := newProjectConfig(t.TempDir(), "shop", "php", "", "", "")
	config.SaveProject(p)
	t.Chdir(p.Root)
	if err := showLogsJSON("web"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"compose", "--env-file", "/dev/null", "-p", "lodev-shop", "-f", composeFile(p), "logs", "-f", "--no-log-prefix", "web"}; !slices.Equal(from, want) {
		t.Errorf("from %v", from)
	}
	if want := []string{"exec", "-i", "lodev-shop-web", "jq", "-R", "-r", "--unbuffered", "fromjson? // ."}; !slices.Equal(to, want) {
		t.Errorf("to %v", to)
	}
	if err := showLogsJSON("mysql"); err == nil || err.Error() != "--json works only with the web service" {
		t.Errorf("mysql: %v", err)
	}
}
