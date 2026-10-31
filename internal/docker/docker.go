// Package docker is a thin wrapper around the docker CLI.
package docker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lodev/assets"
	"lodev/internal/config"
)

func ComposeArgs(project, file string, sub ...string) []string {
	return append([]string{"compose", "-p", project, "-f", file}, sub...)
}

func ExecArgs(project, file, user string, tty bool, cmd []string) []string {
	args := []string{"exec"}
	if !tty {
		args = append(args, "-T")
	}
	args = append(args, "-u", user, "-w", "/var/www/html", "web")
	return ComposeArgs(project, file, append(args, cmd...)...)
}

// PHPBuildArgs builds the base image, labelled with the hash of the assets it came from.
func PHPBuildArgs(versions []string, uid, gid int, hash string, noCache bool, dir string) []string {
	return buildArgs(noCache, "-t", "lodev/php", "--label", "lodev.hash="+hash,
		"--build-arg", "PHP_VERSIONS="+strings.Join(versions, " "),
		"--build-arg", "UID="+strconv.Itoa(uid),
		"--build-arg", "GID="+strconv.Itoa(gid),
		dir)
}

// LegacyBuildArgs builds a legacy image, labelled with the base image ID it was built on.
func LegacyBuildArgs(ver, baseID string, noCache bool, dir string) []string {
	return buildArgs(noCache, "-t", config.PHPImage(ver), "--label", "lodev.base="+baseID,
		"--build-arg", "PHP_VERSION="+ver, "-f", filepath.Join(dir, "Dockerfile.legacy"), dir)
}

func buildArgs(noCache bool, rest ...string) []string {
	if noCache {
		return append([]string{"build", "--no-cache"}, rest...)
	}
	return append([]string{"build"}, rest...)
}

// Run runs docker with the terminal attached.
func Run(args ...string) error {
	c := exec.Command("docker", args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Output runs docker and returns trimmed stdout; stderr is included in the error.
func Output(args ...string) (string, error) {
	var stderr bytes.Buffer
	c := exec.Command("docker", args...)
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func Ping() error {
	if _, err := Output("info", "--format", "{{.ServerVersion}}"); err != nil {
		return errors.New("Docker is not running. Start Docker Desktop.")
	}
	return nil
}

func ImageExists(tag string) bool {
	_, err := Output("image", "inspect", tag)
	return err == nil
}

func Running(container string) bool {
	out, err := Output("inspect", "-f", "{{.State.Running}}", container)
	return err == nil && out == "true"
}

func WaitHealthy(container string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		out, _ := Output("inspect", "-f", "{{.State.Health.Status}}", container)
		if out == "healthy" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s not healthy after %s (status %q); check `lodev services logs`", container, timeout, out)
		}
		time.Sleep(time.Second)
	}
}

// BuildImage builds tag from an embedded assets subtree.
func BuildImage(sub string, args func(dir string) []string) error {
	dir, err := os.MkdirTemp("", "lodev-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := assets.Extract(sub, dir); err != nil {
		return err
	}
	return Run(args(dir)...)
}

// Swapped in tests.
var (
	// inspect formats an image; "" when it does not exist.
	inspect = func(format, tag string) string {
		out, err := Output("image", "inspect", "-f", format, tag)
		if err != nil {
			return ""
		}
		return out
	}
	build   = BuildImage
	phpHash = func() (string, error) { return assets.Hash("php") }
)

// EnsurePHPImage (re)builds the base image when missing, built from other
// assets (e.g. after upgrading lodev) or rebuild, and likewise the legacy
// image for ver when it is missing or sits on an older base.
func EnsurePHPImage(ver string, rebuild bool) error {
	hash, err := phpHash()
	if err != nil {
		return err
	}
	if rebuild || inspect(`{{index .Config.Labels "lodev.hash"}}`, "lodev/php") != hash {
		fmt.Println("Building lodev/php (PHP 8.2-8.5, about a minute the first time)...")
		if err := build("php", func(dir string) []string {
			return PHPBuildArgs(config.BundledPHP, os.Getuid(), os.Getgid(), hash, rebuild, dir)
		}); err != nil {
			return err
		}
	}
	img := config.PHPImage(ver)
	if img == "lodev/php" {
		return nil
	}
	base := inspect("{{.Id}}", "lodev/php")
	if !rebuild && inspect(`{{index .Config.Labels "lodev.base"}}`, img) == base {
		return nil
	}
	fmt.Printf("Building %s...\n", img)
	return build("php", func(dir string) []string { return LegacyBuildArgs(ver, base, rebuild, dir) })
}
