package certs

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSyncTraefik(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	dir := filepath.Join(home, "certs")
	os.MkdirAll(dir, 0o755)
	// Cert names come from .domains files; a project may itself be named *-key.
	for _, f := range []string{"a.pem", "a-key.pem", "a.domains", "api-key.pem", "api-key-key.pem", "api-key.domains"} {
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	if err := SyncTraefik(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "dynamic.yaml"))
	s := string(data)
	if strings.Count(s, "certFile:") != 2 || !strings.Contains(s, "certFile: /certs/a.pem") || !strings.Contains(s, "certFile: /certs/api-key.pem") {
		t.Fatal(s)
	}
}

// noMkcert hides a host mkcert so CAFile is LODEV_HOME/ca/rootCA.pem.
func noMkcert(t *testing.T, home string) string {
	t.Setenv("PATH", t.TempDir())
	return filepath.Join(home, "ca", "rootCA.pem") + "\n"
}

func TestEnsureSkipsWhenUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	ca := noMkcert(t, home)
	dir := filepath.Join(home, "certs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "shop.pem"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "shop.domains"), []byte(ca+"a.test b.test"), 0o644)
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
	ca := noMkcert(t, home)
	dir := filepath.Join(home, "certs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "shop.pem"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "shop.domains"), []byte(ca+"a.test"), 0o644)
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
	if d, _ := os.ReadFile(filepath.Join(dir, "shop.domains")); string(d) != ca+"a.test c.test" {
		t.Fatalf("domains file %q", d)
	}
}

func TestEnsureRegeneratesWhenCAChanges(t *testing.T) {
	// Certs signed by lodev's dockerized CA must be reissued once mkcert's CA is used.
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	noMkcert(t, home)
	dir := filepath.Join(home, "certs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "shop.pem"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "shop.domains"), []byte("/other/ca/rootCA.pem\na.test"), 0o644)
	orig := mkcertRun
	t.Cleanup(func() { mkcertRun = orig })
	called := false
	mkcertRun = func(args ...string) error { called = true; return nil }
	if err := Ensure("shop", []string{"a.test"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("cert from another CA was kept")
	}
}

// fakeMkcert puts an mkcert script first on PATH whose CAROOT already has rootCA.pem.
func fakeMkcert(t *testing.T, body string) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	bin, caroot := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(caroot, "rootCA.pem"), nil, 0o644)
	script := "#!/bin/sh\nif [ \"$1\" = -CAROOT ]; then echo " + caroot + "; exit 0; fi\n" + body
	os.WriteFile(filepath.Join(bin, "mkcert"), []byte(script), 0o755)
	t.Setenv("PATH", bin)
	t.Setenv("LODEV_HOME", t.TempDir())
	os.MkdirAll(dir(), 0o755)
}

// captureStd returns what fn wrote to os.Stdout and os.Stderr.
func captureStd(t *testing.T, fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	fn()
	os.Stdout, os.Stderr = oldOut, oldErr
	w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestRunMkcertQuietOnSuccess(t *testing.T) {
	fakeMkcert(t, "echo 'Note: the local CA is not installed in the system trust store.' >&2\n")
	got := captureStd(t, func() {
		if err := runMkcert("-cert-file", "a.pem"); err != nil {
			t.Error(err)
		}
	})
	if got != "" {
		t.Fatalf("printed %q", got)
	}
}

func TestRunMkcertErrorIncludesOutput(t *testing.T) {
	fakeMkcert(t, "echo 'ERROR: bad domain' >&2; exit 1\n")
	var err error
	captureStd(t, func() { err = runMkcert("x") })
	if err == nil || !strings.Contains(err.Error(), "ERROR: bad domain") {
		t.Fatal(err)
	}
}
