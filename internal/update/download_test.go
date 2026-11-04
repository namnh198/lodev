package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testArchive = "lodev_1.3.0_linux_amd64.tar.gz"

// release builds a tar.gz; names without "/" go under the archive's top folder.
func release(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if !strings.HasPrefix(name, "../") {
			name = "lodev_1.3.0_linux_amd64/" + name
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// serveRelease serves tgz and checksums (sum of tgz when checksums is "") under /v1.3.0/.
func serveRelease(t *testing.T, tgz []byte, checksums string) {
	t.Helper()
	if checksums == "" {
		checksums = fmt.Sprintf("%x  %s\n", sha256.Sum256(tgz), testArchive)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.3.0/" + testArchive:
			if tgz != nil {
				w.Write(tgz)
				return
			}
		case "/v1.3.0/checksums.txt":
			w.Write([]byte(checksums))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	old := DownloadBase
	DownloadBase = srv.URL
	t.Cleanup(func() { DownloadBase = old })
}

func download(t *testing.T) (string, error) {
	t.Helper()
	dir, err := Download(context.Background(), "v1.3.0", "linux", "amd64")
	if dir != "" {
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	return dir, err
}

func TestDownload(t *testing.T) {
	serveRelease(t, release(t, map[string]string{"lodev": "bin", "completions/_lodev": "zsh"}), "")
	dir, err := download(t)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "lodev")); string(b) != "bin" {
		t.Errorf("lodev = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "completions", "_lodev")); string(b) != "zsh" {
		t.Errorf("_lodev = %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "completions", "lodev.bash")); !os.IsNotExist(err) {
		t.Errorf("lodev.bash should be absent: %v", err)
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	serveRelease(t, release(t, map[string]string{"lodev": "bin"}), strings.Repeat("0", 64)+"  "+testArchive+"\n")
	dir, err := download(t)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") || dir != "" {
		t.Errorf("got %q, %v", dir, err)
	}
}

func TestDownloadExactChecksumName(t *testing.T) {
	tgz := release(t, map[string]string{"lodev": "bin"})
	serveRelease(t, tgz, fmt.Sprintf("%x  %s.sig\n", sha256.Sum256(tgz), testArchive))
	if _, err := download(t); err == nil || !strings.Contains(err.Error(), "not listed in checksums.txt") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadNoBinary(t *testing.T) {
	serveRelease(t, release(t, map[string]string{"README.md": "x"}), "")
	if _, err := download(t); err == nil {
		t.Error("want error for an archive without lodev")
	}
}

func TestDownloadIgnoresOtherEntries(t *testing.T) {
	serveRelease(t, release(t, map[string]string{"lodev": "bin", "README.md": "x", "../evil": "x"}), "")
	dir, err := download(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(dir, "evil"), filepath.Join(filepath.Dir(dir), "evil"), filepath.Join(dir, "README.md")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should not exist", p)
		}
	}
}

func TestDownload404(t *testing.T) {
	serveRelease(t, nil, "x")
	if dir, err := download(t); err == nil || dir != "" {
		t.Errorf("got %q, %v", dir, err)
	}
}
