package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DownloadBase is where release files live: <DownloadBase>/<tag>/<file>.
var DownloadBase = "https://github.com/namnh198/lodev/releases/download"

const maxFile = 100 << 20

// DownloadClient fetches release files; the timeout keeps a stalled network from hanging forever.
var DownloadClient = &http.Client{Timeout: 10 * time.Minute}

// releasePublicKey is the base64 ed25519 public key that checksums.txt.sig must verify against;
// made by `go run ./tools/sign -genkey`. Rotating it is a new lodev release. Empty: no download is trusted.
var releasePublicKey = ""

// errNotFound marks a 404 from fetch.
var errNotFound = errors.New("not found")

// archiveFiles maps the files read from a release archive to where Download puts them.
var archiveFiles = map[string]string{
	"lodev":                  "lodev",
	"completions/lodev.bash": filepath.Join("completions", "lodev.bash"),
	"completions/_lodev":     filepath.Join("completions", "_lodev"),
	"completions/lodev.fish": filepath.Join("completions", "lodev.fish"),
}

// Download fetches the release archive for tag, checks checksums.txt against its signature and the
// archive's sha256 against checksums.txt, then extracts lodev and its completions into a new temp
// dir, which the caller removes.
func Download(ctx context.Context, tag, goos, goarch string) (dir string, err error) {
	if _, err := publicKey(releasePublicKey); err != nil {
		return "", err
	}
	name := fmt.Sprintf("lodev_%s_%s_%s", strings.TrimPrefix(tag, "v"), goos, goarch)
	tgz, err := fetch(ctx, tag, name+".tar.gz")
	if err != nil {
		return "", err
	}
	sums, err := fetch(ctx, tag, "checksums.txt")
	if err != nil {
		return "", err
	}
	sig, err := fetch(ctx, tag, "checksums.txt.sig")
	if errors.Is(err, errNotFound) {
		return "", unsigned(tag)
	}
	if err != nil {
		return "", err
	}
	if err := verifySignature(sums, sig); err != nil {
		return "", unsigned(tag)
	}
	if err := verify(tgz, sums, name+".tar.gz"); err != nil {
		return "", err
	}
	if dir, err = os.MkdirTemp("", "lodev-upgrade-"); err != nil {
		return "", err
	}
	if err := extract(tgz, name, dir); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func fetch(ctx context.Context, tag, file string) ([]byte, error) {
	u := DownloadBase + "/" + tag + "/" + file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := DownloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("download %s: HTTP 404: %w", u, errNotFound)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("download %s: HTTP %d", u, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFile+1))
	if err == nil && len(data) > maxFile {
		err = fmt.Errorf("download %s: larger than 100 MB", u)
	}
	return data, err
}

func unsigned(tag string) error {
	return fmt.Errorf("release %s is not signed by lodev's release key; install older releases with install.sh or your package manager", tag)
}

// publicKey decodes the base64 key; an empty one means this build was made without a release key.
func publicKey(b64 string) (ed25519.PublicKey, error) {
	if b64 == "" {
		return nil, errors.New("this lodev build has no release key")
	}
	k, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(k) != ed25519.PublicKeySize {
		return nil, errors.New("this lodev build has an invalid release key")
	}
	return k, nil
}

// verifySignature checks sig (base64 of the raw signature, whitespace around it ignored) over sums.
func verifySignature(sums, sig []byte) error {
	key, err := publicKey(releasePublicKey)
	if err != nil {
		return err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || !ed25519.Verify(key, sums, raw) { // Verify is false for a wrong-length signature
		return errors.New("bad signature")
	}
	return nil
}

func verify(data, sums []byte, file string) error {
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == file {
			got := sha256.Sum256(data)
			if hex.EncodeToString(got[:]) != f[0] {
				return fmt.Errorf("checksum mismatch for %s", file)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not listed in checksums.txt", file)
}

// extract writes only the archiveFiles entries, to fixed paths: no archive path is used to write.
func extract(tgz []byte, name, dir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		dst, ok := archiveFiles[strings.TrimPrefix(h.Name, name+"/")]
		if !ok || h.Typeflag != tar.TypeReg || !strings.HasPrefix(h.Name, name+"/") {
			continue
		}
		if h.Size > maxFile {
			return fmt.Errorf("%s: larger than 100 MB", h.Name)
		}
		dst = filepath.Join(dir, dst)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(tr, maxFile))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "lodev")); err != nil {
		return fmt.Errorf("%s.tar.gz has no lodev binary", name)
	}
	return nil
}
