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
	"path/filepath"
	"strconv"
	"strings"

	"lodev/assets"
	"lodev/internal/config"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

func ComposeArgs(project, file string, sub ...string) []string {
	return append([]string{"compose", "--env-file", os.DevNull, "-p", project, "-f", file}, sub...)
}

// Both pass --env-file: compose would otherwise read .env next to the first file, which is
// .lodev/.env, and fail on a value like pa${ss (lodev puts its values in the compose file).

// ComposeConfigArgs merges compose files into one document on stdout.
func ComposeConfigArgs(project string, files []string) []string {
	args := []string{"compose", "--env-file", os.DevNull, "-p", project}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	return append(args, "config")
}

// ExecArgs execs in a compose service; empty user/workdir keep the service's defaults.
func ExecArgs(project, file, service, user, workdir string, tty bool, cmd []string) []string {
	args := []string{"exec"}
	if !tty {
		args = append(args, "-T")
	}
	if user != "" {
		args = append(args, "-u", user)
	}
	if workdir != "" {
		args = append(args, "-w", workdir)
	}
	args = append(args, service)
	return ComposeArgs(project, file, append(args, cmd...)...)
}

// PHPBuildArgs builds the base image, labelled with the hash of the assets it came from.
func PHPBuildArgs(versions []string, node, hash string, noCache bool, dir string) []string {
	return buildArgs(noCache, "-t", config.WebserverImage(), "--label", "lodev.hash="+hash,
		"--build-arg", "PHP_VERSIONS="+strings.Join(versions, " "), "--build-arg", "NODE_VERSION="+node,
		dir)
}

// LegacyBuildArgs builds a legacy image, labelled with the base image ID it was built on.
func LegacyBuildArgs(ver, baseID string, noCache bool, dir string) []string {
	return buildArgs(noCache, "-t", config.PHPImage(ver), "--label", "lodev.base="+baseID,
		"--build-arg", "PHP_VERSION="+ver, "--build-arg", "BASE="+config.WebserverImage(), "-f", filepath.Join(dir, "Dockerfile.legacy"), dir)
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

// joinArgs renders args for the debug line, quoting only those containing a space.
func joinArgs(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = a
		if strings.Contains(a, " ") {
			q[i] = strconv.Quote(a)
		}
	}
	return strings.Join(q, " ")
}

// Run runs docker with the terminal attached.
func Run(args ...string) error {
	c, err := command(args)
	if err != nil {
		return err
	}
	ui.Debug("$ %s %s", filepath.Base(c.Path), joinArgs(c.Args[1:]))
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Pipe runs docker from | docker to, the terminal on to's stdout and both stderrs.
func Pipe(from, to []string) error {
	a, err := command(from)
	if err != nil {
		return err
	}
	b, err := command(to)
	if err != nil {
		return err
	}
	ui.Debug("$ %s %s | %s %s", filepath.Base(a.Path), joinArgs(a.Args[1:]), filepath.Base(b.Path), joinArgs(b.Args[1:]))
	if b.Stdin, err = a.StdoutPipe(); err != nil {
		return err
	}
	a.Stderr, b.Stdout, b.Stderr = os.Stderr, os.Stdout, os.Stderr
	if err := b.Start(); err != nil {
		return err
	}
	errA := a.Run()
	if errB := b.Wait(); errB != nil {
		return errB
	}
	return errA
}

// Output runs docker and returns trimmed stdout; stderr is included in the error.
func Output(args ...string) (string, error) {
	c, err := command(args)
	if err != nil {
		return "", err
	}
	prog := filepath.Base(c.Path)
	ui.Debug("$ %s %s", prog, joinArgs(c.Args[1:]))
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", prog, strings.Join(c.Args[1:], " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func Ping() error {
	if _, err := Output("info", "--format", "{{.ServerVersion}}"); err != nil {
		return errors.New("Docker is not running: start Docker Desktop (or the docker service) and try again")
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
	phpHash = func() (string, error) { return assets.Hash("webserver") }
)

// EnsurePHPImage (re)builds the base image when missing, built from other
// assets (e.g. after upgrading lodev) or rebuild, and likewise the legacy
// image for ver when it is missing or sits on an older base. node is the Node.js it installs.
func EnsurePHPImage(ver, node string, rebuild bool) error {
	hash, err := phpHash()
	if err != nil {
		return err
	}
	hash += "+node" + node // a new node_version rebuilds the image too
	base := config.WebserverImage()
	if rebuild || inspect(`{{index .Config.Labels "lodev.hash"}}`, base) != hash {
		ui.Info("Building %s (PHP 8.2-8.5, about a minute the first time)...", base)
		if err := build("webserver", func(dir string) []string {
			return PHPBuildArgs(config.BundledPHP, node, hash, rebuild, dir)
		}); err != nil {
			return err
		}
	}
	img := config.PHPImage(ver)
	if img == base {
		return nil
	}
	baseID := inspect("{{.Id}}", base)
	if !rebuild && inspect(`{{index .Config.Labels "lodev.base"}}`, img) == baseID {
		return nil
	}
	ui.Info("Building %s...", img)
	return build("webserver", func(dir string) []string { return LegacyBuildArgs(ver, baseID, rebuild, dir) })
}

// WebImageParts reads the top-level Dockerfile* files of .lodev/web-images, sorted, skipping *.example.
func WebImageParts(root string) ([]templates.UserPart, error) {
	dir := filepath.Join(root, ".lodev", "web-images")
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var parts []templates.UserPart
	for _, e := range entries { // ReadDir is sorted by name
		n := e.Name()
		if !strings.HasPrefix(n, "Dockerfile") || strings.HasSuffix(n, ".example") || !e.Type().IsRegular() {
			continue
		}
		f := filepath.Join(dir, n)
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
// returns the image tag compose should run; otherwise the plain PHP image
// (a stale project image is removed by the caller once compose has moved off it).
func EnsureProjectImage(p *config.Project, rebuild bool) (string, error) {
	tag := config.ProjectImage(p.Name)
	parts, err := WebImageParts(p.Root)
	if err != nil {
		return "", err
	}
	if !NeedsProjectImage(p, parts) {
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
		ui.Info("Building %s...", tag)
		if err := run(ProjectBuildArgs(tag, hash, dfPath, ctx, rebuild)...); err != nil {
			return "", fmt.Errorf("building %s failed; check .lodev/web-images/ and php_extensions/packages: %w", tag, err)
		}
	}
	return tag, nil
}
