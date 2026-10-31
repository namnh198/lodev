// Package docker is a thin wrapper around the docker CLI.
package docker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// PHPImage is the image a project on PHP ver runs.
func PHPImage(ver string) string {
	if slices.Contains(config.BundledPHP, ver) {
		return "lodev/php"
	}
	// legacy- keeps clear of the old per-version lodev/php:<ver> images.
	return "lodev/php:legacy-" + ver
}

func PHPBuildArgs(versions []string, uid, gid int, dir string) []string {
	return []string{"build", "-t", "lodev/php",
		"--build-arg", "PHP_VERSIONS=" + strings.Join(versions, " "),
		"--build-arg", "UID=" + strconv.Itoa(uid),
		"--build-arg", "GID=" + strconv.Itoa(gid),
		dir}
}

func LegacyBuildArgs(ver, dir string) []string {
	return []string{"build", "-t", PHPImage(ver), "--build-arg", "PHP_VERSION=" + ver,
		"-f", filepath.Join(dir, "Dockerfile.legacy"), dir}
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

// EnsurePHPImage builds the base image, and the legacy image for ver, if missing or rebuild.
func EnsurePHPImage(ver string, rebuild bool) error {
	if rebuild || !ImageExists("lodev/php") {
		fmt.Println("Building lodev/php (PHP 8.2-8.5, first time takes a few minutes)...")
		if err := BuildImage("php", func(dir string) []string {
			return PHPBuildArgs(config.BundledPHP, os.Getuid(), os.Getgid(), dir)
		}); err != nil {
			return err
		}
	}
	img := PHPImage(ver)
	if img == "lodev/php" || (!rebuild && ImageExists(img)) {
		return nil
	}
	fmt.Printf("Building %s...\n", img)
	return BuildImage("php", func(dir string) []string { return LegacyBuildArgs(ver, dir) })
}
