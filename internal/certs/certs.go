// Package certs issues local TLS certificates with mkcert (host binary or Docker fallback).
package certs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

func dir() string { return filepath.Join(config.Home(), "certs") }

// caDir holds lodev's own CA. Older lodev kept it in LODEV_HOME/ca: it is moved, so a CA the
// user already trusts stays in use.
func caDir() string {
	d, old := filepath.Join(config.Home(), "rootCA"), filepath.Join(config.Home(), "ca")
	if !exists(d) && exists(old) {
		if err := os.Rename(old, d); err != nil {
			return old
		}
	}
	return d
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// mkcertRun runs mkcert inside the certs dir; swapped in tests.
var mkcertRun = runMkcert

func runMkcert(args ...string) error {
	if path, err := exec.LookPath("mkcert"); err == nil {
		caroot, err := exec.Command(path, "-CAROOT").Output()
		if err != nil {
			return err
		}
		if !exists(filepath.Join(strings.TrimSpace(string(caroot)), "rootCA.pem")) {
			if err := attached(exec.Command(path, "-install")); err != nil {
				return err
			}
		}
		c := exec.Command(path, args...)
		c.Dir = dir()
		// Captured: mkcert's notes (e.g. "the local CA is not installed") are noise on every start.
		out, err := c.CombinedOutput()
		ui.Debug("%s", out)
		if err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	for _, d := range []string{caDir(), dir()} { // Docker would create a missing bind source as root
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	fresh := !exists(filepath.Join(caDir(), "rootCA.pem"))
	if !docker.ImageExists("lodev/mkcert") {
		err := docker.BuildImage("mkcert", func(d string) []string { return []string{"build", "-t", "lodev/mkcert", d} })
		if err != nil {
			return err
		}
	}
	// --hostname: mkcert names the CA "mkcert <user>@<hostname>"; a container ID would say nothing.
	run := append([]string{"run", "--rm", "--hostname", "lodev-mkcert", "-e", "CAROOT=/ca", "-v", caDir() + ":/ca", "-v", dir() + ":/certs", "-w", "/certs", "lodev/mkcert"}, args...)
	out, err := docker.Output(run...)
	ui.Debug("%s", out)
	if err != nil {
		return err
	}
	if fresh {
		ca := filepath.Join(caDir(), "rootCA.pem")
		fmt.Println()
		ui.Info("%s", trustHint(runtime.GOOS, ca))
		fmt.Println()
	}
	return nil
}

// EnsureCA creates the CA when it is missing (first run, or deleted since): lodev's own one
// without mkcert, mkcert's own one (installed into the trust store) with it.
func EnsureCA() error {
	ca, err := CAFile()
	if err != nil || exists(ca) {
		return err
	}
	// Host mkcert: runMkcert installs the missing CA first. lodev's container: -install creates it.
	return mkcertRun("-install")
}

// caSum identifies the CA by content: a deleted and recreated CA keeps its path but signs anew.
func caSum(ca string) string {
	b, err := os.ReadFile(ca)
	if err != nil {
		return ""
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// trustHint tells how to trust lodev's own CA on goos (browsers warn until it is).
func trustHint(goos, ca string) string {
	h := "mkcert is not installed, so lodev created its own local CA. To remove browser warnings:\n"
	switch goos {
	case "darwin":
		h += "  macOS (Chrome, Safari):\n    sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain " + ca + "\n"
	case "linux":
		h += "  System (curl, PHP, …), one of:\n" +
			"    Debian/Ubuntu: sudo cp " + ca + " /usr/local/share/ca-certificates/lodev.crt && sudo update-ca-certificates\n" +
			"    Fedora/RHEL:   sudo cp " + ca + " /etc/pki/ca-trust/source/anchors/lodev.pem && sudo update-ca-trust\n" +
			"    Arch:          sudo trust anchor --store " + ca + "\n" +
			"  Chrome/Chromium (needs certutil: libnss3-tools / nss-tools):\n" +
			`    certutil -d sql:$HOME/.pki/nssdb -A -t "C,," -n lodev -i ` + ca + "\n" +
			"  Firefox, each profile with a cert9.db:\n" +
			`    for d in ~/.mozilla/firefox/*/ ~/snap/firefox/common/.mozilla/firefox/*/; do [ -f "$d/cert9.db" ] && certutil -d "sql:$d" -A -t "C,," -n lodev -i ` + ca + "; done\n" +
			"  Or install mkcert (and certutil) and run `mkcert -install`: lodev then uses its CA.\n"
		return h
	case "windows":
		h += "  Windows (as administrator):\n    certutil -addstore -f ROOT " + ca + "\n"
	}
	return h + "  Firefox: Settings → Certificates → Import " + ca
}

func attached(c *exec.Cmd) error {
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// CAFile is the root CA that containers must trust.
func CAFile() (string, error) {
	if path, err := exec.LookPath("mkcert"); err == nil {
		out, err := exec.Command(path, "-CAROOT").Output()
		if err != nil {
			return "", err
		}
		return filepath.Join(strings.TrimSpace(string(out)), "rootCA.pem"), nil
	}
	return filepath.Join(caDir(), "rootCA.pem"), nil
}

// Ensure issues <name>.pem for domains unless the existing cert already covers them.
func Ensure(name string, domains []string) error {
	if err := EnsureCA(); err != nil {
		return err
	}
	ca, err := CAFile()
	if err != nil {
		return err
	}
	// The CA (path and content) is part of the key: certs are reissued when mkcert's CA replaces
	// lodev's own, or when the CA was deleted and created again.
	want := ca + "\n" + caSum(ca) + "\n" + strings.Join(domains, " ")
	domainsFile := filepath.Join(dir(), name+".domains")
	if got, err := os.ReadFile(domainsFile); err == nil && string(got) == want && exists(filepath.Join(dir(), name+".pem")) {
		return nil
	}
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	args := append([]string{"-cert-file", name + ".pem", "-key-file", name + "-key.pem"}, domains...)
	if err := mkcertRun(args...); err != nil {
		return fmt.Errorf("mkcert: %w", err)
	}
	return os.WriteFile(domainsFile, []byte(want), 0o644)
}

func Remove(name string) error {
	for _, f := range []string{name + ".pem", name + "-key.pem", name + ".domains"} {
		if err := os.Remove(filepath.Join(dir(), f)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// SyncTraefik rewrites Traefik's certs.yaml to list every issued cert.
func SyncTraefik() error {
	// <name>.domains exists for every issued cert; *.pem would also match key files.
	files, err := filepath.Glob(filepath.Join(dir(), "*.domains"))
	if err != nil {
		return err
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".domains"))
	}
	sort.Strings(names)
	data, err := templates.TraefikDynamic(names)
	if err != nil {
		return err
	}
	if err := config.WriteFileAtomic(filepath.Join(config.TraefikDir(), "certs.yaml"), data, 0o644); err != nil {
		return err
	}
	// Older lodev kept the certs file here.
	if err := os.Remove(filepath.Join(dir(), "dynamic.yaml")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
