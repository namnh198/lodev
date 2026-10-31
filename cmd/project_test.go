package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/registry"
)

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

func TestEnsureGitignore(t *testing.T) {
	root := t.TempDir()
	want := ".lodev/compose.yaml\n.lodev/webserver.conf\n"
	ensureGitignore(root)
	if b, _ := os.ReadFile(filepath.Join(root, ".gitignore")); string(b) != want {
		t.Fatalf("%q", b)
	}

	root = t.TempDir()
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("vendor/"), 0o644)
	ensureGitignore(root)
	ensureGitignore(root)
	if b, _ := os.ReadFile(filepath.Join(root, ".gitignore")); string(b) != "vendor/\n"+want {
		t.Fatalf("%q", b)
	}
}

func TestNewProjectConfigEmptyNameHint(t *testing.T) {
	_, err := newProjectConfig(filepath.Join(t.TempDir(), "___"), "", "php", "", "")
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("want a --name hint, got %v", err)
	}
}
