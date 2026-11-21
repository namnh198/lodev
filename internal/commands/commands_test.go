package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/templates"
)

func TestParseHeader(t *testing.T) {
	src := templates.GeneratedMarker + "\n# Usage: magento [args...]\n# Description: Run bin/magento in the web container\n# Type: magento\nexec php bin/magento \"$@\"\n"
	c, err := Parse("magento", Web, "", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if c.Use != "magento [args...]" || c.Short != "Run bin/magento in the web container" ||
		!reflect.DeepEqual(c.Types, []string{"magento"}) || !c.Marker {
		t.Fatalf("got %+v", c)
	}
	c, err = Parse("x", Web, "", []byte("# Type: laravel , magento\n"))
	if err != nil || !reflect.DeepEqual(c.Types, []string{"laravel", "magento"}) {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestParseDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, err := Parse("foo", Host, filepath.Join(home, "x", "foo"), []byte("echo hi\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Use != "foo [args...]" || c.Short != "Custom command (~/x/foo)" || c.Types != nil || c.Marker {
		t.Fatalf("got %+v", c)
	}
	c, _ = Parse("foo", Web, "", []byte("# Usage: bar baz\n"))
	if c.Use != "foo [args...]" {
		t.Fatalf("Use = %q", c.Use)
	}
}

func TestParseShebang(t *testing.T) {
	m := templates.GeneratedMarker
	for _, c := range []struct {
		src        string
		interp, mk bool
	}{
		{"#!/usr/bin/env php\necho 1;\n", true, false},
		{"#!/usr/bin/python3\n", true, false},
		{"#!/bin/sh\n", true, false},
		{"#!/usr/bin/env bash\n", false, false},
		{"#!/bin/bash\n", false, false},
		{"#!/usr/bin/bash\n", false, false},
		{"echo hi\n", false, false},
		{"#!\n", false, false},
		{"#!/usr/bin/env bash\n" + m + "\necho\n", false, true},
		{"#!/usr/bin/env php\n" + m + "\n", true, true},
		{m + "\necho\n", false, true},
		{"#!/usr/bin/env bash\n# x\n" + m + "\n", false, false},
		{"# x\n" + m + "\n", false, false},
	} {
		got, err := Parse("foo", Web, "", []byte(c.src))
		if err != nil || got.Interp != c.interp || got.Marker != c.mk {
			t.Errorf("%q: Interp %v Marker %v, want %v %v (%v)", c.src, got.Interp, got.Marker, c.interp, c.mk, err)
		}
	}
}

func TestParseUnknownType(t *testing.T) {
	_, err := Parse("foo", Web, "", []byte("# Type: drupal\n"))
	if err == nil || !strings.Contains(err.Error(), "drupal") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseCRLF(t *testing.T) {
	c, err := Parse("foo", Web, "", []byte("# Description: d\r\necho hi\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Short != "d" || strings.Contains(c.Script, "\r") {
		t.Fatalf("got %+v", c)
	}
}

func TestApplies(t *testing.T) {
	if !(Command{}).Applies("php") || !(Command{Types: []string{"php"}}).Applies("php") ||
		(Command{Types: []string{"magento"}}).Applies("php") {
		t.Fatal("Applies wrong")
	}
}

func TestValidName(t *testing.T) {
	for _, n := range []string{"composer", "db-gui", "9x"} {
		if !ValidName(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range []string{"Foo", "-x", "a.sh", "a_b", ""} {
		if ValidName(n) {
			t.Errorf("%q should be invalid", n)
		}
	}
}

func TestLoadDir(t *testing.T) {
	if cmds, w := LoadDir(filepath.Join(t.TempDir(), "none"), Web); cmds != nil || w != nil {
		t.Fatalf("missing dir: %v, %v", cmds, w)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"ok": "echo ok\n", "bad.name": "x\n", "x.example": "x\n", ".hidden": "x\n", "typo": "# Type: nope\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmds, warns := LoadDir(dir, Web)
	if len(cmds) != 1 || cmds[0].Name != "ok" || cmds[0].Kind != Web || cmds[0].Path != filepath.Join(dir, "ok") {
		t.Fatalf("cmds = %+v", cmds)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings = %q", warns)
	}
	for _, n := range []string{"bad.name", "typo"} {
		got := 0
		for _, w := range warns {
			if strings.Contains(w, n) {
				got++
			}
		}
		if got != 1 {
			t.Errorf("%q should be in exactly one warning: %q", n, warns)
		}
	}
}

func TestShortPath(t *testing.T) {
	t.Setenv("HOME", "/Users/u")
	for in, want := range map[string]string{
		"/Users/u/a/b": "~/a/b",
		"/Users/u":     "~",
		"/Users/uX/p":  "/Users/uX/p",
		"/opt/x":       "/opt/x",
	} {
		if got := ShortPath(in); got != want {
			t.Errorf("ShortPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostEnv(t *testing.T) {
	g, _ := config.ParseGlobal(nil)
	g.Ports.MySQL = "3306"
	p := &config.Project{Name: "shop", Type: "php", Root: "/r", Webserver: "nginx", Domains: []string{"shop.test"}, Services: []string{"mysql"}}
	env := HostEnv(p, g, "/r/app", "/bin/lodev", true)
	for _, e := range []string{"LODEV_BIN=/bin/lodev", "LODEV_PWD=/r/app", "LODEV_TYPE=php", "LODEV_PROJECT_STATUS=running",
		"LODEV_DB_TYPE=mysql", "LODEV_DB_HOST=127.0.0.1", "LODEV_DB_PORT=3306", "LODEV_DB_USER=root", "LODEV_DB_PASSWORD=root",
		"LODEV_DB_NAME=shop", "LODEV_WEB_CONTAINER=lodev-shop-web", "LODEV_URL=https://shop.test"} {
		if !slices.Contains(env, e) {
			t.Errorf("env lacks %s: %q", e, env)
		}
	}
	for _, e := range env {
		if k, _, _ := strings.Cut(e, "="); !slices.Contains(HostEnvKeys, k) {
			t.Errorf("HostEnvKeys lacks %s", k)
		}
	}
	p.Services, p.Webserver = nil, "none"
	for _, e := range HostEnv(p, g, "/r", "/bin/lodev", false) {
		if strings.HasPrefix(e, "LODEV_DB_") {
			t.Errorf("no db, got %s", e)
		}
		if strings.HasPrefix(e, "LODEV_URL=") && e != "LODEV_URL=" {
			t.Errorf("webserver none: %s", e)
		}
	}
	if env := HostEnv(nil, g, "/x", "/bin/lodev", false); !slices.Equal(env, []string{"LODEV_BIN=/bin/lodev", "LODEV_PWD=/x"}) {
		t.Errorf("outside a project: %q", env)
	}
}
