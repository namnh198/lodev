package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
	c, _ = Parse("foo", Web, "", []byte("#!/bin/bash\n"+templates.GeneratedMarker+"\n"))
	if c.Marker {
		t.Fatal("marker on line 2 must not count")
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
