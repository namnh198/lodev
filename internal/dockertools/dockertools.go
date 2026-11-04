// Package dockertools downloads the pinned docker-compose and docker-buildx binaries lodev runs,
// so every machine uses the same versions whatever Docker plugins it has.
package dockertools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"lodev/internal/config"
	"lodev/internal/update"
)

type Tool int

const (
	Compose Tool = iota
	Buildx
)

// Pinned versions; a lodev release bumps them.
const (
	ComposeVersion = "v5.7.0"
	BuildxVersion  = "v0.38.2"
)

// Release download bases, <base>/<version>/<asset>; swapped in tests.
var (
	ComposeBase = "https://github.com/docker/compose/releases/download"
	BuildxBase  = "https://github.com/docker/buildx/releases/download"
)

// Where the download line and progress bar go; swapped in tests.
var (
	progressOut io.Writer = os.Stderr
	isTerminal            = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }
)

func (t Tool) Name() string {
	if t == Buildx {
		return "docker-buildx"
	}
	return "docker-compose"
}

func (t Tool) Version() string {
	if t == Buildx {
		return BuildxVersion
	}
	return ComposeVersion
}

func (t Tool) base() string {
	if t == Buildx {
		return BuildxBase
	}
	return ComposeBase
}

// Asset is the release file for goos/goarch.
func Asset(t Tool, goos, goarch string) string {
	if t == Buildx {
		return fmt.Sprintf("buildx-%s.%s-%s", BuildxVersion, goos, goarch)
	}
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	return fmt.Sprintf("docker-compose-%s-%s", goos, arch)
}

// File is where lodev keeps t; the name has no version, a newer pin replaces it.
func File(t Tool) string { return filepath.Join(config.Home(), "bin", t.Name()) }

// Installed is the version File(t) reports ("v5.7.0"), or "" when it is missing or does not run.
func Installed(t Tool) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args := []string{"version", "--short"} // compose: "5.7.0"
	if t == Buildx {
		args = []string{"version"} // "github.com/docker/buildx v0.38.2 <commit>"
	}
	out, err := exec.CommandContext(ctx, File(t), args...).Output()
	if err != nil {
		return ""
	}
	if t == Buildx {
		if f := strings.Fields(string(out)); len(f) > 1 {
			return f[1]
		}
		return ""
	}
	if v := strings.TrimSpace(string(out)); v != "" {
		return "v" + strings.TrimPrefix(v, "v")
	}
	return ""
}

// Path is File(t), downloaded first when it is missing or another version.
func Path(t Tool) (string, error) {
	if Installed(t) == t.Version() {
		return File(t), nil
	}
	if err := download(t); err != nil {
		return "", err
	}
	return File(t), nil
}

func download(t Tool) error {
	asset := Asset(t, runtime.GOOS, runtime.GOARCH)
	sum, err := checksum(t, asset)
	if err != nil {
		return err
	}
	dir := filepath.Dir(File(t))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	resp, err := get(t, asset)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	tmp, err := os.CreateTemp(dir, "."+t.Name()+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), wrapProgress(t, resp.ContentLength, resp.Body))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != sum {
		return fmt.Errorf("checksum mismatch for %s", asset)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), File(t))
}

// checksum finds asset in checksums.txt; buildx lists its code-signed macOS binaries only in
// checksums-signed.txt.
func checksum(t Tool, asset string) (string, error) {
	files := []string{"checksums.txt"}
	if t == Buildx {
		files = append(files, "checksums-signed.txt")
	}
	for _, f := range files {
		resp, err := get(t, f)
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if fs := strings.Fields(line); len(fs) == 2 && strings.TrimPrefix(fs[1], "*") == asset {
				return fs[0], nil
			}
		}
	}
	return "", fmt.Errorf("%s is not listed in the %s %s checksums", asset, t.Name(), t.Version())
}

func get(t Tool, file string) (*http.Response, error) {
	u := t.base() + "/" + t.Version() + "/" + file
	resp, err := update.DownloadClient.Get(u)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d", u, resp.StatusCode)
	}
	return resp, nil
}
