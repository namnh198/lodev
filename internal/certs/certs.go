// Package certs issues local TLS certificates with mkcert (host binary or Docker fallback).
package certs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

func dir() string   { return filepath.Join(config.Home(), "certs") }
func caDir() string { return filepath.Join(config.Home(), "ca") }

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
		return attached(c)
	}

	if err := os.MkdirAll(caDir(), 0o755); err != nil {
		return err
	}
	fresh := !exists(filepath.Join(caDir(), "rootCA.pem"))
	if !docker.ImageExists("lodev/mkcert") {
		err := docker.BuildImage("mkcert", func(d string) []string { return []string{"build", "-t", "lodev/mkcert", d} })
		if err != nil {
			return err
		}
	}
	run := append([]string{"run", "--rm", "-e", "CAROOT=/ca", "-v", caDir() + ":/ca", "-v", dir() + ":/certs", "-w", "/certs", "lodev/mkcert"}, args...)
	if err := docker.Run(run...); err != nil {
		return err
	}
	if fresh {
		ca := filepath.Join(caDir(), "rootCA.pem")
		fmt.Println()
		ui.Info(`mkcert is not installed, so lodev created its own local CA. To remove browser warnings:
  macOS/Chrome/Safari:
    sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s
  Firefox: Settings → Certificates → Import %s`, ca, ca)
		fmt.Println()
	}
	return nil
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
	ca, err := CAFile()
	if err != nil {
		return err
	}
	// The CA is part of the key: certs from lodev's own CA are reissued once mkcert is used.
	want := ca + "\n" + strings.Join(domains, " ")
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

// SyncTraefik rewrites Traefik's dynamic config to list every issued cert.
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
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir(), "dynamic.yaml"), data, 0o644)
}
