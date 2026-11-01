// Package docker is a thin wrapper around the docker CLI.
package docker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"lodev/assets"
	"lodev/internal/config"
	"lodev/internal/templates"
)

func ComposeArgs(project, file string, sub ...string) []string {
	return append([]string{"compose", "-p", project, "-f", file}, sub...)
}

// ComposeConfigArgs merges compose files into one document on stdout.
func ComposeConfigArgs(project string, files []string) []string {
	args := []string{"compose", "-p", project}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	return append(args, "config")
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

// ProjectBuildArgs builds the per-project web image, labelled with its input hash.
func ProjectBuildArgs(tag, hash, dockerfile, ctx string, noCache bool) []string {
	return buildArgs(noCache, "-t", tag, "--label", "lodev.hash="+hash, "-f", dockerfile, ctx)
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
	run     = Run
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

// WebImageParts reads the top-level Dockerfile* files of .lodev/web-images, sorted, skipping *.example.
func WebImageParts(root string) ([]templates.UserPart, error) {
	files, err := filepath.Glob(filepath.Join(root, ".lodev", "web-images", "Dockerfile*"))
	if err != nil {
		return nil, err
	}
	var parts []templates.UserPart
	for _, f := range files { // Glob returns sorted names
		if strings.HasSuffix(f, ".example") {
			continue
		}
		if fi, err := os.Stat(f); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		parts = append(parts, templates.UserPart{Name: ".lodev/web-images/" + filepath.Base(f), Content: b})
	}
	return parts, nil
}

// NeedsProjectImage reports whether the project customizes its web image.
func NeedsProjectImage(p *config.Project, parts []templates.UserPart) bool {
	return p.NodeJS != "" || len(p.PHPExtensions) > 0 || len(p.Packages) > 0 || len(parts) > 0
}

// projectHash covers everything that goes into the project image build.
func projectHash(dockerfile []byte, baseID, ctxDir string) (string, error) {
	h := sha256.New()
	h.Write(dockerfile)
	h.Write([]byte(baseID))
	err := fs.WalkDir(os.DirFS(ctxDir), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		b, err := os.ReadFile(filepath.Join(ctxDir, path))
		if err != nil {
			return err
		}
		h.Write([]byte(path + "\x00"))
		h.Write(b)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) { // no web-images dir
		err = nil
	}
	return hex.EncodeToString(h.Sum(nil)), err
}

// EnsureProjectImage builds the project's web image when it needs one and
// returns the image tag compose should run; otherwise it drops any stale
// image and returns the plain PHP image.
func EnsureProjectImage(p *config.Project, rebuild bool) (string, error) {
	tag := config.ProjectImage(p.Name)
	parts, err := WebImageParts(p.Root)
	if err != nil {
		return "", err
	}
	if !NeedsProjectImage(p, parts) {
		if inspect("{{.Id}}", tag) != "" {
			_ = run("rmi", tag)
		}
		return config.PHPImage(p.PHP), nil
	}
	df, err := templates.WebBuildDockerfile(p, parts)
	if err != nil {
		return "", err
	}
	buildDir := filepath.Join(p.Root, ".lodev", "web-build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return "", err
	}
	dfPath := filepath.Join(buildDir, "Dockerfile")
	if err := os.WriteFile(dfPath, df, 0o644); err != nil {
		return "", err
	}
	imgDir := filepath.Join(p.Root, ".lodev", "web-images")
	ctx := imgDir
	if fi, err := os.Stat(imgDir); err != nil || !fi.IsDir() {
		ctx = buildDir
	}
	hash, err := projectHash(df, inspect("{{.Id}}", config.PHPImage(p.PHP)), imgDir)
	if err != nil {
		return "", err
	}
	if rebuild || inspect(`{{index .Config.Labels "lodev.hash"}}`, tag) != hash {
		fmt.Printf("Building %s...\n", tag)
		if err := run(ProjectBuildArgs(tag, hash, dfPath, ctx, rebuild)...); err != nil {
			return "", fmt.Errorf("building %s failed; check .lodev/web-images/ and php_extensions/packages: %w", tag, err)
		}
	}
	return tag, nil
}
