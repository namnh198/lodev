package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

// signWith returns a signature body for sums, as CI writes it: base64 plus a newline.
func signWith(priv ed25519.PrivateKey, sums string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(sums))) + "\n"
}

// testKey makes releasePublicKey a fresh test key for the test and returns its private half.
func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := releasePublicKey
	releasePublicKey = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { releasePublicKey = old })
	return priv
}

// serveRelease serves tgz and checksums (sum of tgz when checksums is "") under /v1.3.0/,
// with a valid checksums.txt.sig from a fresh test key.
func serveRelease(t *testing.T, tgz []byte, checksums string) {
	t.Helper()
	priv := testKey(t)
	serveReleaseSig(t, tgz, checksums, func(sums string) (string, bool) { return signWith(priv, sums), true })
}

// serveReleaseSig is serveRelease with the signature chosen by sig; ok=false means no .sig file.
// It returns the paths requested so far.
func serveReleaseSig(t *testing.T, tgz []byte, checksums string, sig func(sums string) (string, bool)) *[]string {
	t.Helper()
	if checksums == "" {
		checksums = fmt.Sprintf("%x  %s\n", sha256.Sum256(tgz), testArchive)
	}
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1.3.0/" + testArchive:
			if tgz != nil {
				w.Write(tgz)
				return
			}
		case "/v1.3.0/checksums.txt":
			w.Write([]byte(checksums))
			return
		case "/v1.3.0/checksums.txt.sig":
			if body, ok := sig(checksums); ok {
				w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	old := DownloadBase
	DownloadBase = srv.URL
	t.Cleanup(func() { DownloadBase = old })
	return &paths
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

func TestDownloadTimesOut(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer func() { close(release); srv.Close() }()
	testKey(t) // a key is set, so the stalled server is what Download waits on
	defer func(b string, c *http.Client) { DownloadBase, DownloadClient = b, c }(DownloadBase, DownloadClient)
	DownloadBase, DownloadClient = srv.URL, &http.Client{Timeout: 100 * time.Millisecond}
	done := make(chan error, 1)
	go func() { _, err := Download(context.Background(), "v1.3.0", "linux", "amd64"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("want a timeout error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Download still hangs on a stalled server")
	}
}

func TestDownloadClientHasTimeout(t *testing.T) {
	if DownloadClient.Timeout == 0 {
		t.Error("DownloadClient must time out so a stalled network cannot hang self-upgrade")
	}
}

const notSigned = "release v1.3.0 is not signed by lodev's release key"

func TestDownloadRejectsUnsigned(t *testing.T) {
	testKey(t)
	got := serveReleaseSig(t, release(t, map[string]string{"lodev": "bin"}), "", func(string) (string, bool) { return "", false })
	if dir, err := download(t); err == nil || !strings.Contains(err.Error(), notSigned) || dir != "" {
		t.Errorf("got %q, %v", dir, err)
	}
	// Fail fast: the signature is checked before the archive is downloaded.
	if slices.Contains(*got, "/v1.3.0/"+testArchive) {
		t.Errorf("archive downloaded for an unsigned release: %v", *got)
	}
}

func TestDownloadRejectsBadSignature(t *testing.T) {
	testKey(t)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	for name, sig := range map[string]func(string) string{
		"other key":    func(s string) string { return signWith(other, s) },
		"garbage":      func(string) string { return "not base64!\n" },
		"wrong length": func(string) string { return base64.StdEncoding.EncodeToString([]byte("short")) },
		"empty":        func(string) string { return "" },
	} {
		t.Run(name, func(t *testing.T) {
			serveReleaseSig(t, release(t, map[string]string{"lodev": "bin"}), "", func(s string) (string, bool) { return sig(s), true })
			if dir, err := download(t); err == nil || !strings.Contains(err.Error(), notSigned) || dir != "" {
				t.Errorf("got %q, %v", dir, err)
			}
		})
	}
}

// The signature covers checksums.txt: a valid signature of other bytes does not vouch for these.
func TestDownloadRejectsTamperedChecksums(t *testing.T) {
	priv := testKey(t)
	tgz := release(t, map[string]string{"lodev": "evil"})
	forged := fmt.Sprintf("%x  %s\n", sha256.Sum256(tgz), testArchive)
	serveReleaseSig(t, tgz, forged, func(string) (string, bool) { return signWith(priv, "the real checksums\n"), true })
	if _, err := download(t); err == nil || !strings.Contains(err.Error(), notSigned) {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadAcceptsSigned(t *testing.T) {
	priv := testKey(t)
	serveReleaseSig(t, release(t, map[string]string{"lodev": "bin"}), "", func(s string) (string, bool) {
		return "  " + signWith(priv, s) + "\n", true // surrounding whitespace is tolerated
	})
	if _, err := download(t); err != nil {
		t.Fatal(err)
	}
}

func TestDownloadNoReleaseKey(t *testing.T) {
	serveRelease(t, release(t, map[string]string{"lodev": "bin"}), "")
	releasePublicKey = ""
	if dir, err := download(t); err == nil || err.Error() != "this lodev build has no release key" || dir != "" {
		t.Errorf("got %q, %v", dir, err)
	}
}

func TestCommittedReleaseKeyIsEmptyOrValid(t *testing.T) {
	if releasePublicKey == "" {
		return
	}
	if _, err := publicKey(releasePublicKey); err != nil {
		t.Error(err)
	}
}
