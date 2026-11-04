package dockertools

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	composeScript = "#!/bin/sh\necho 5.7.0\n"
	buildxScript  = "#!/bin/sh\necho github.com/docker/buildx v0.38.2 abc\n"
)

type release struct {
	requests int
	asset    string
	body     string
	sums     string // checksums.txt
	signed   string // checksums-signed.txt
	missing  bool   // 404 for the asset
}

// fakeRelease serves t's pinned asset; LODEV_HOME is a temp dir and the progress output is discarded.
func fakeRelease(t *testing.T, tool Tool, body string) *release {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	r := &release{asset: Asset(tool, runtime.GOOS, runtime.GOARCH), body: body}
	r.sums = fmt.Sprintf("%x *%s\n", sha256.Sum256([]byte(body)), r.asset)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.requests++
		switch req.URL.Path {
		case "/" + tool.Version() + "/" + r.asset:
			if r.missing {
				http.NotFound(w, req)
				return
			}
			w.Write([]byte(r.body))
		case "/" + tool.Version() + "/checksums.txt":
			w.Write([]byte(r.sums))
		case "/" + tool.Version() + "/checksums-signed.txt":
			w.Write([]byte(r.signed))
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(srv.Close)
	base := &ComposeBase
	if tool == Buildx {
		base = &BuildxBase
	}
	old, oldOut := *base, progressOut
	*base, progressOut = srv.URL, &strings.Builder{}
	t.Cleanup(func() { *base, progressOut = old, oldOut })
	return r
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestAsset(t *testing.T) {
	for _, c := range []struct {
		tool       Tool
		goos, arch string
		want       string
	}{
		{Compose, "darwin", "arm64", "docker-compose-darwin-aarch64"},
		{Compose, "linux", "amd64", "docker-compose-linux-x86_64"},
		{Buildx, "darwin", "arm64", "buildx-v0.38.2.darwin-arm64"},
		{Buildx, "linux", "amd64", "buildx-v0.38.2.linux-amd64"},
	} {
		if got := Asset(c.tool, c.goos, c.arch); got != c.want {
			t.Errorf("Asset(%s, %s/%s) = %q, want %q", c.tool.Name(), c.goos, c.arch, got, c.want)
		}
	}
}

func TestPathDownloadsOnce(t *testing.T) {
	r := fakeRelease(t, Compose, composeScript)
	p, err := Path(Compose)
	if err != nil {
		t.Fatal(err)
	}
	if p != File(Compose) || filepath.Base(p) != "docker-compose" {
		t.Errorf("path %q", p)
	}
	if b, _ := os.ReadFile(p); string(b) != composeScript {
		t.Errorf("content %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o755 {
		t.Errorf("mode %v", fi.Mode())
	}
	if got := Installed(Compose); got != "v5.7.0" {
		t.Errorf("Installed = %q", got)
	}
	n := r.requests
	if _, err := Path(Compose); err != nil || r.requests != n {
		t.Errorf("second Path: %v, %d new requests", err, r.requests-n)
	}
}

func TestPathReplacesOtherVersion(t *testing.T) {
	for name, old := range map[string]string{"old version": "#!/bin/sh\necho 5.6.0\n", "broken": "#!/bin/sh\nexit 1\n"} {
		t.Run(name, func(t *testing.T) {
			fakeRelease(t, Compose, composeScript)
			writeScript(t, File(Compose), old)
			if _, err := Path(Compose); err != nil {
				t.Fatal(err)
			}
			if got := Installed(Compose); got != "v5.7.0" {
				t.Errorf("Installed = %q", got)
			}
		})
	}
}

func TestPathChecksumMismatch(t *testing.T) {
	r := fakeRelease(t, Compose, composeScript)
	r.sums = strings.Repeat("0", 64) + " *" + r.asset + "\n"
	if _, err := Path(Compose); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
	if left := tempFiles(t); len(left) > 0 || fileExists(File(Compose)) {
		t.Errorf("left behind: %v", left)
	}
}

func TestPathFailureKeepsOldFile(t *testing.T) {
	r := fakeRelease(t, Compose, composeScript)
	r.missing = true
	old := "#!/bin/sh\necho 5.6.0\n"
	writeScript(t, File(Compose), old)
	if _, err := Path(Compose); err == nil {
		t.Fatal("want an error for a missing asset")
	}
	if b, _ := os.ReadFile(File(Compose)); string(b) != old {
		t.Errorf("old file changed: %q", b)
	}
	if left := tempFiles(t); len(left) > 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestPathNotListed(t *testing.T) {
	r := fakeRelease(t, Compose, composeScript)
	r.sums = "abc *something-else\n"
	if _, err := Path(Compose); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("err = %v", err)
	}
}

// buildx publishes the code-signed macOS binaries' sums only in checksums-signed.txt.
func TestPathBuildxSignedChecksums(t *testing.T) {
	r := fakeRelease(t, Buildx, buildxScript)
	r.signed, r.sums = r.sums, "abc *buildx-v0.38.2.other\n"
	if _, err := Path(Buildx); err != nil {
		t.Fatal(err)
	}
	if got := Installed(Buildx); got != "v0.38.2" {
		t.Errorf("Installed = %q", got)
	}
}

// A network that drops GitHub traffic must not make every lodev command wait again.
func TestPathFailureBacksOff(t *testing.T) {
	r := fakeRelease(t, Compose, composeScript)
	r.missing = true
	if _, err := Path(Compose); err == nil {
		t.Fatal("want an error")
	}
	n := r.requests
	_, err := Path(Compose)
	if err == nil || !strings.Contains(err.Error(), "download failed recently") || !strings.Contains(err.Error(), "HTTP 404") || r.requests != n {
		t.Fatalf("second Path: %v, %d new requests", err, r.requests-n)
	}
	// After the back-off it tries again, and a success clears the stamp.
	old := time.Now().Add(-2 * time.Hour)
	os.Chtimes(failedStamp(Compose), old, old)
	r.missing = false
	if _, err := Path(Compose); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(failedStamp(Compose)); !os.IsNotExist(err) {
		t.Error("stamp kept after a successful download")
	}
}

func TestClientFailsFast(t *testing.T) {
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.ResponseHeaderTimeout == 0 || tr.ResponseHeaderTimeout > 15*time.Second || tr.TLSHandshakeTimeout == 0 || tr.Proxy == nil {
		t.Errorf("transport %+v", client.Transport)
	}
	if client.Timeout == 0 {
		t.Error("no overall timeout")
	}
}

// tempFiles are partial downloads (.docker-compose-*), not the .failed stamp.
func tempFiles(t *testing.T) []string {
	t.Helper()
	all, _ := filepath.Glob(filepath.Join(filepath.Dir(File(Compose)), ".docker-compose-*"))
	return all
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// The checksum fetch can fail before bin/ exists; the stamp must still be written.
func TestPathFailureBeforeBinDir(t *testing.T) {
	r := fakeRelease(t, Compose, composeScript)
	r.sums = "" // asset not listed: fails before any file is written
	Path(Compose)
	if !fileExists(failedStamp(Compose)) {
		t.Error("no stamp after an early failure")
	}
}
