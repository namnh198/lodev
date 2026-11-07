package certs

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
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

// noMkcert hides a host mkcert so CAFile is LODEV_HOME/rootCA/rootCA.pem.
func noMkcert(t *testing.T, home string) string {
	t.Setenv("PATH", t.TempDir())
	return filepath.Join(home, "rootCA", "rootCA.pem") + "\n"
}

// withCA writes lodev's CA and returns the .domains prefix for it: path and content hash.
func withCA(t *testing.T, home string) string {
	t.Helper()
	ca := filepath.Join(strings.TrimSuffix(noMkcert(t, home), "\n"))
	os.MkdirAll(filepath.Dir(ca), 0o755)
	os.WriteFile(ca, []byte("ca-content"), 0o644)
	sum := sha256.Sum256([]byte("ca-content"))
	return ca + "\n" + hex.EncodeToString(sum[:]) + "\n"
}

func TestEnsureSkipsWhenUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	ca := withCA(t, home)
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
	ca := withCA(t, home)
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

func TestCAFileMovesOldCADir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	want := noMkcert(t, home)
	os.MkdirAll(filepath.Join(home, "ca"), 0o755)
	os.WriteFile(filepath.Join(home, "ca", "rootCA.pem"), []byte("trusted"), 0o644)
	got, err := CAFile()
	if err != nil || got+"\n" != want {
		t.Fatalf("CAFile = %q, %v; want %q", got, err, want)
	}
	if b, _ := os.ReadFile(got); string(b) != "trusted" {
		t.Fatalf("old CA not moved: %q", b)
	}
	if exists(filepath.Join(home, "ca")) {
		t.Fatal("old ca dir still there")
	}
}

func TestEnsureCA(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	noMkcert(t, home)
	orig := mkcertRun
	t.Cleanup(func() { mkcertRun = orig })
	var calls [][]string
	mkcertRun = func(args ...string) error { calls = append(calls, args); return nil }
	if err := EnsureCA(); err != nil || len(calls) != 1 || strings.Join(calls[0], " ") != "-install" {
		t.Fatalf("no CA: calls %q, err %v", calls, err)
	}
	os.MkdirAll(filepath.Join(home, "rootCA"), 0o755)
	os.WriteFile(filepath.Join(home, "rootCA", "rootCA.pem"), nil, 0o644)
	if err := EnsureCA(); err != nil || len(calls) != 1 {
		t.Fatalf("CA exists: calls %q, err %v", calls, err)
	}
}

// A deleted CA must be recreated and the certs it signed reissued: the path alone does not change.
func TestEnsureReissuesWhenCAIsGone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	noMkcert(t, home)
	ca := filepath.Join(home, "rootCA", "rootCA.pem")
	orig := mkcertRun
	t.Cleanup(func() { mkcertRun = orig })
	var calls []string
	mkcertRun = func(args ...string) error { // like mkcert: creates the CA when it is missing
		calls = append(calls, strings.Join(args, " "))
		os.MkdirAll(filepath.Dir(ca), 0o755)
		if _, err := os.Stat(ca); err != nil {
			os.WriteFile(ca, []byte("CA-"+strconv.Itoa(len(calls))), 0o644)
		}
		os.WriteFile(filepath.Join(home, "certs", "shop.pem"), nil, 0o644)
		return nil
	}
	if err := Ensure("shop", []string{"a.test"}); err != nil {
		t.Fatal(err)
	}
	n := len(calls) // -install (no CA yet), then the cert
	if err := Ensure("shop", []string{"a.test"}); err != nil || len(calls) != n {
		t.Fatalf("unchanged: mkcert ran again: %q, %v", calls[n:], err)
	}
	os.RemoveAll(filepath.Dir(ca))
	if err := Ensure("shop", []string{"a.test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ca); err != nil || !slices.Equal(calls[n:], []string{"-install", "-cert-file shop.pem -key-file shop-key.pem a.test"}) {
		t.Fatalf("CA not recreated or cert not reissued: %v, calls %q", err, calls[n:])
	}
	os.WriteFile(ca, []byte("another CA"), 0o644) // replaced, e.g. restored from elsewhere
	n = len(calls)
	if err := Ensure("shop", []string{"a.test"}); err != nil || len(calls) != n+1 {
		t.Fatalf("cert not reissued for a new CA: calls %q, %v", calls, err)
	}
}

func TestTrustHint(t *testing.T) {
	ca := "/h/.lodev/rootCA/rootCA.pem"
	mac := trustHint("darwin", ca)
	for _, want := range []string{"sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain " + ca, "Firefox"} {
		if !strings.Contains(mac, want) {
			t.Errorf("darwin hint lacks %q:\n%s", want, mac)
		}
	}
	linux := trustHint("linux", ca)
	for _, want := range []string{
		"sudo cp " + ca + " /usr/local/share/ca-certificates/lodev.crt && sudo update-ca-certificates",
		"sudo cp " + ca + " /etc/pki/ca-trust/source/anchors/lodev.pem && sudo update-ca-trust",
		"sudo trust anchor --store " + ca,
		`certutil -d sql:$HOME/.pki/nssdb -A -t "C,," -n lodev -i ` + ca,
		"cert9.db",
		"mkcert -install",
	} {
		if !strings.Contains(linux, want) {
			t.Errorf("linux hint lacks %q:\n%s", want, linux)
		}
	}
	if strings.Contains(linux, "security add-trusted-cert") {
		t.Errorf("linux hint shows the macOS command:\n%s", linux)
	}
	if w := trustHint("windows", ca); !strings.Contains(w, "certutil -addstore -f ROOT "+ca) {
		t.Errorf("windows hint:\n%s", w)
	}
}
