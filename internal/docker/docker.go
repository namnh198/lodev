// Package docker is a thin wrapper around the docker CLI.
package docker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"lodev/assets"
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

func PHPBuildArgs(ver string, uid, gid int, dir string) []string {
	return []string{"build", "-t", "lodev/php:" + ver,
		"--build-arg", "PHP_VERSION=" + ver,
		"--build-arg", "UID=" + strconv.Itoa(uid),
		"--build-arg", "GID=" + strconv.Itoa(gid),
		dir}
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

func EnsurePHPImage(ver string, rebuild bool) error {
	if !rebuild && ImageExists("lodev/php:"+ver) {
		return nil
	}
	fmt.Printf("Building lodev/php:%s (first time takes a few minutes)...\n", ver)
	return BuildImage("php", func(dir string) []string {
		return PHPBuildArgs(ver, os.Getuid(), os.Getgid(), dir)
	})
}
