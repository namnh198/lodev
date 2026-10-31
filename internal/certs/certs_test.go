package certs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncTraefik(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	dir := filepath.Join(home, "certs")
	os.MkdirAll(dir, 0o755)
	for _, f := range []string{"a.pem", "a-key.pem", "b.pem", "b-key.pem", "a.domains"} {
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	if err := SyncTraefik(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "dynamic.yaml"))
	s := string(data)
	if strings.Count(s, "certFile:") != 2 || !strings.Contains(s, "certFile: /certs/a.pem") || !strings.Contains(s, "certFile: /certs/b.pem") {
		t.Fatal(s)
	}
}

func TestEnsureSkipsWhenUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	dir := filepath.Join(home, "certs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "shop.pem"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "shop.domains"), []byte("a.test b.test"), 0o644)
	orig := mkcertRun
	t.Cleanup(func() { mkcertRun = orig })
	mkcertRun = func(args ...string) error { t.Fatal("mkcert called"); return nil }
	if err := Ensure("shop", []string{"a.test", "b.test"}); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRegeneratesWhenDomainsChange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	dir := filepath.Join(home, "certs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "shop.pem"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "shop.domains"), []byte("a.test"), 0o644)
	orig := mkcertRun
	t.Cleanup(func() { mkcertRun = orig })
	var got []string
	mkcertRun = func(args ...string) error { got = args; return nil }
	if err := Ensure("shop", []string{"a.test", "c.test"}); err != nil {
		t.Fatal(err)
	}
	want := "-cert-file shop.pem -key-file shop-key.pem a.test c.test"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %v", got)
	}
	if d, _ := os.ReadFile(filepath.Join(dir, "shop.domains")); string(d) != "a.test c.test" {
		t.Fatalf("domains file %q", d)
	}
}
