// Package dockertools downloads the pinned docker-compose and docker-buildx binaries lodev runs,
// so every machine uses the same versions whatever Docker plugins it has.
package dockertools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"lodev/internal/config"
)

type Tool int

const (
	Compose Tool = iota
	Buildx
)

// Pinned versions; a lodev release bumps them together with pinnedSums.
const (
	ComposeVersion = "v5.7.0"
	BuildxVersion  = "v0.38.2"
)

// pinnedSums is the sha256 of each pinned asset, copied from the releases' checksums.txt (buildx's
// macOS ones from checksums-signed.txt). Kept here, not fetched: a release's checksums.txt cannot
// vouch for an asset served next to it. Swapped in tests.
var pinnedSums = map[string]string{
	"docker-compose-darwin-aarch64": "54495ff81dd8cae11861455f0308a293a530149beba8f1d90ff3f295b11cf035",
	"docker-compose-darwin-x86_64":  "f57fad1fc3a942309b76da99e0a1c49dc97f7c56d4a03593d71bb01577aeeb29",
	"docker-compose-linux-aarch64":  "15861535d14e26555736b49376c95e14bfd94285366c8446bfac7156a972e76f",
	"docker-compose-linux-x86_64":   "7758048ad38e580f22559603312874fec8bb638d021a9e64182385fc17cc12a2",
	"buildx-v0.38.2.darwin-amd64":   "ad2418546e6d168a3e3b59fdf2a24997bc84263abeb4a5d9d0422cfe782743fc",
	"buildx-v0.38.2.darwin-arm64":   "81702e18713cfd495bfe594943fa57f470eda16c0653ae3de15cd411df273c41",
	"buildx-v0.38.2.linux-amd64":    "7992ec09413b80ff57ba4e081f56fc833d7bac1d5e71143c6c809e7e6065f6d6",
	"buildx-v0.38.2.linux-arm64":    "46c0f334772de3cb0b341cc415caf7339fb5cfc8b8438ebcb5a10e7f23800d36",
}

// Release download bases, <base>/<version>/<asset>; swapped in tests.
var (
	ComposeBase = "https://github.com/docker/compose/releases/download"
	BuildxBase  = "https://github.com/docker/buildx/releases/download"
)

// client fails fast on a network that silently drops GitHub traffic; the overall timeout bounds the body.
var client = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	},
}

// retryAfter is how long a failed download is not retried, so a blocked network costs one wait.
const retryAfter = time.Hour

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
	stamp := failedStamp(t)
	if fi, err := os.Stat(stamp); err == nil && time.Since(fi.ModTime()) < retryAfter {
		reason, _ := os.ReadFile(stamp)
		return "", fmt.Errorf("download failed recently (%s); retrying after %s", strings.TrimSpace(string(reason)), fi.ModTime().Add(retryAfter).Format("15:04"))
	}
	if err := download(t); err != nil {
		// Best effort: without the stamp the next command just retries.
		if os.MkdirAll(filepath.Dir(stamp), 0o755) == nil {
			os.WriteFile(stamp, []byte(err.Error()), 0o644)
		}
		return "", err
	}
	os.Remove(stamp)
	return File(t), nil
}

// failedStamp records the last failed download of t (its time and reason).
func failedStamp(t Tool) string {
	return filepath.Join(filepath.Dir(File(t)), "."+t.Name()+".failed")
}

func download(t Tool) error {
	asset := Asset(t, runtime.GOOS, runtime.GOARCH)
	sum, ok := pinnedSums[asset]
	if !ok {
		return fmt.Errorf("%s %s has no pinned checksum for %s", t.Name(), t.Version(), asset)
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

func get(t Tool, file string) (*http.Response, error) {
	u := t.base() + "/" + t.Version() + "/" + file
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d", u, resp.StatusCode)
	}
	return resp, nil
}
