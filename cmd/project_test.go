package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/hosts"
	"lodev/internal/registry"
	"lodev/internal/templates"
)

func TestExtraComposeFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".lodev")
	os.MkdirAll(dir, 0o755)
	for _, n := range []string{"b.compose.yaml", "a.compose.yml", "x.compose.yml.example", "lodev.compose-base.yml"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	got, err := extraComposeFiles(&config.Project{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.compose.yml"), filepath.Join(dir, "b.compose.yaml")}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMergeCompose(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
	p := &config.Project{Name: "x", Root: root}
	full := composeFile(p)
	os.WriteFile(full, []byte("old"), 0o644)
	orig := composeConfig
	defer func() { composeConfig = orig }()

	composeConfig = func(...string) (string, error) { return "", errors.New("boom") }
	if err := mergeCompose(p); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(full); string(b) != "old" {
		t.Fatalf("full file clobbered: %q", b)
	}

	composeConfig = func(...string) (string, error) { return "name: x", nil }
	if err := mergeCompose(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(full); string(b) != "name: x\n" {
		t.Fatalf("got %q", b)
	}
}

func TestNewProjectConfigDetects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "My_Shop.v2")
	os.MkdirAll(filepath.Join(root, "bin"), 0o755)
	os.WriteFile(filepath.Join(root, "bin", "magento"), nil, 0o644)
	p, err := newProjectConfig(root, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "my-shop-v2" || p.Type != "magento" || p.Docroot != "pub" || p.PHP != "8.3" ||
		p.Webserver != "nginx" || !slices.Equal(p.Services, []string{"mysql", "redis", "opensearch"}) || p.Root != root {
		t.Fatalf("%+v", p)
	}
}

func TestNewProjectConfigFlagsWin(t *testing.T) {
	p, err := newProjectConfig(t.TempDir(), "x", "laravel", "7.4", "none")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "x" || p.Type != "laravel" || p.PHP != "7.4" || p.Webserver != "none" || p.Docroot != "public" ||
		!slices.Equal(p.Services, []string{"mysql", "redis"}) {
		t.Fatalf("%+v", p)
	}
}

func TestNewProjectConfigRejectsInvalid(t *testing.T) {
	if _, err := newProjectConfig(t.TempDir(), "x", "php", "7.3", ""); err == nil {
		t.Fatal("php 7.3 accepted")
	}
}

func TestDeleteRefusesCopyOfAnotherProject(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	orig := t.TempDir()
	os.MkdirAll(filepath.Join(orig, ".lodev"), 0o755)
	os.WriteFile(filepath.Join(orig, ".lodev", "config.yaml"), nil, 0o644)
	reg := registry.Registry{"shop": {Path: orig, Domains: []string{"shop.test"}}}
	reg.Save()

	copyP, _ := newProjectConfig(t.TempDir(), "shop", "php", "", "")
	err := deleteProject(copyP, true, true)
	if err == nil || !strings.Contains(err.Error(), orig) {
		t.Fatalf("delete of a copy must refuse and name the original: %v", err)
	}
	if got, _ := registry.Load(); got["shop"].Path != orig {
		t.Fatal("original registry entry was removed")
	}
}

func TestDeleteRemovesProjectImage(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	hf := filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(hf, []byte("127.0.0.1 localhost\n"), 0o644)
	defer func(o string) { hosts.File = o }(hosts.File)
	hosts.File = hf
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)

	p, _ := newProjectConfig(t.TempDir(), "shop", "php", "8.3", "")
	var got []string
	for _, rmiErr := range []error{nil, errors.New("Error response from daemon: No such image: lodev/shop-web:latest")} {
		got = nil
		dockerOutput = func(a ...string) (string, error) { got = a; return "", rmiErr }
		if err := deleteProject(p, false, true); err != nil {
			t.Fatalf("delete failed with rmi err %v: %v", rmiErr, err)
		}
		if strings.Join(got, " ") != "rmi lodev/shop-web" {
			t.Fatalf("docker call = %v", got)
		}
	}
	dockerOutput = func(...string) (string, error) { return "", errors.New("conflict: image in use") }
	if err := deleteProject(p, false, true); err == nil {
		t.Fatal("other rmi errors must surface")
	}
}

func confProject(t *testing.T, ws string) *config.Project {
	t.Helper()
	p, err := newProjectConfig(t.TempDir(), "app", "php", "8.3", ws)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func writeConf(t *testing.T, p *config.Project, body string) {
	t.Helper()
	os.MkdirAll(filepath.Join(p.Root, ".lodev"), 0o755)
	if err := os.WriteFile(WebserverConfFile(p), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteWebserverConfRegenerates(t *testing.T) {
	p := confProject(t, "nginx")
	writeConf(t, p, templates.GeneratedMarker+"\nold\n")
	if err := writeWebserverConf(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(WebserverConfFile(p)); strings.Contains(string(b), "old\n") || !strings.HasPrefix(string(b), templates.GeneratedMarker) {
		t.Fatalf("%q", b)
	}
}

func TestWriteWebserverConfUserManaged(t *testing.T) {
	for name, body := range map[string]string{
		"no marker":           "mine\n",
		"marker not 1st line": "# mine\n" + templates.GeneratedMarker + "\nx\n",
	} {
		p := confProject(t, "nginx")
		writeConf(t, p, body)
		if err := writeWebserverConf(p); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(WebserverConfFile(p)); string(b) != body {
			t.Fatalf("%s: %q", name, b)
		}
	}
}

func TestWriteWebserverConfSwitch(t *testing.T) {
	p := confProject(t, "nginx")
	writeConf(t, p, "mine\n")
	p.Webserver = "apache"
	if err := writeWebserverConf(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(WebserverConfFile(p)); !strings.HasPrefix(string(b), templates.GeneratedMarker) {
		t.Fatalf("apache conf: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(p.Root, ".lodev", "nginx-webserver.conf")); string(b) != "mine\n" {
		t.Fatalf("nginx conf changed: %q", b)
	}
}

func TestWriteLodevGitignore(t *testing.T) {
	p := confProject(t, "nginx")
	want := "# Generated by lodev on every `lodev start`; do not edit.\n/lodev.compose-base.yml\n/lodev.compose-full.yml\n/web-build/\n" +
		"# each <ws>-webserver.conf below is listed only while it carries the marker\n/nginx-webserver.conf\n/apache-webserver.conf\n"
	gi := filepath.Join(p.Root, ".lodev", ".gitignore")
	if err := writeLodevGitignore(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(gi); string(b) != want {
		t.Fatalf("%q", b)
	}
	writeConf(t, p, "mine\n")
	if err := writeLodevGitignore(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(gi); string(b) != strings.Replace(want, "/nginx-webserver.conf\n", "", 1) {
		t.Fatalf("%q", b)
	}
	if _, err := os.Stat(filepath.Join(p.Root, ".gitignore")); err == nil {
		t.Fatal("root .gitignore created")
	}
}

func TestNewProjectConfigEmptyNameHint(t *testing.T) {
	_, err := newProjectConfig(filepath.Join(t.TempDir(), "___"), "", "php", "", "")
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("want a --name hint, got %v", err)
	}
}
