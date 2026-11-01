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
		"composer": "composer", "npm": "npm", "node": "node",
		"artisan": "php artisan", "wp": "wp", "magento": "php bin/magento",
	}
	for name, want := range cases {
		if got := shortcutCmd(name); !slices.Equal(got, strings.Fields(want)) {
			t.Errorf("%s: %v", name, got)
		}
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
