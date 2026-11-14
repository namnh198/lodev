package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignRoundTrip(t *testing.T) {
	priv, pub, err := genKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SIGNING_KEY", priv)
	file := filepath.Join(t.TempDir(), "checksums.txt")
	data := []byte("abc  lodev_1.0.0_linux_amd64.tar.gz\n")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sign("TEST_SIGNING_KEY", file); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file + ".sig")
	if err != nil || !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("sig file %q, %v", raw, err)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	pubRaw, _ := base64.StdEncoding.DecodeString(pub)
	if len(priv) == 0 || !ed25519.Verify(pubRaw, data, sig) {
		t.Error("signature does not verify against the printed public key")
	}
}

func TestSignRejectsMissingOrInvalidKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "checksums.txt")
	os.WriteFile(file, []byte("x"), 0o644)
	t.Setenv("TEST_SIGNING_KEY", "")
	if err := sign("TEST_SIGNING_KEY", file); err == nil || !strings.Contains(err.Error(), "TEST_SIGNING_KEY") {
		t.Errorf("unset: %v", err)
	}
	t.Setenv("TEST_SIGNING_KEY", "not a key")
	if err := sign("TEST_SIGNING_KEY", file); err == nil {
		t.Error("invalid key: want error")
	}
	if _, err := os.Stat(file + ".sig"); !os.IsNotExist(err) {
		t.Errorf("no .sig must be written on failure: %v", err)
	}
}
