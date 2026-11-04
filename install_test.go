package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testArchive = "lodev_9.9.9_linux_amd64"

// fakeRelease writes <dir>/v9.9.9/{<archive>.tar.gz,checksums.txt}; a wrong sum breaks the checksum.
func fakeRelease(t *testing.T, wrongSum bool) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"lodev":                  "#!/bin/sh\necho fake lodev\n",
		"completions/lodev.bash": "bash", "completions/_lodev": "zsh", "completions/lodev.fish": "fish",
		"LICENSE": "MIT License\n",
	} {
		tw.WriteHeader(&tar.Header{Name: testArchive + "/" + name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	sum := fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
	if wrongSum {
		sum = strings.Repeat("0", 64)
	}
	dir := t.TempDir()
	rel := filepath.Join(dir, "v9.9.9")
	os.MkdirAll(rel, 0o755)
	os.WriteFile(filepath.Join(rel, testArchive+".tar.gz"), buf.Bytes(), 0o644)
	os.WriteFile(filepath.Join(rel, "checksums.txt"), []byte(sum+"  "+testArchive+".tar.gz\n"), 0o644)
	return dir
}

func runInstall(t *testing.T, release, prefix string) (string, error) {
	t.Helper()
	c := exec.Command("sh", "install.sh")
	c.Env = append(os.Environ(), "VERSION=v9.9.9", "PREFIX="+prefix, "BASE_URL=file://"+release,
		"LODEV_UNAME_S=Linux", "LODEV_UNAME_M=x86_64")
	out, err := c.CombinedOutput()
	return string(out), err
}

func TestInstallScript(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "my apps") // a space must survive every path and hint
	out, err := runInstall(t, fakeRelease(t, false), prefix)
	if err != nil {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	for f, mode := range map[string]os.FileMode{
		"bin/lodev": 0o755,
		"share/bash-completion/completions/lodev":    0,
		"share/zsh/site-functions/_lodev":            0,
		"share/fish/vendor_completions.d/lodev.fish": 0,
		"share/doc/lodev/LICENSE":                    0,
	} {
		fi, err := os.Stat(filepath.Join(prefix, f))
		if err != nil {
			t.Errorf("missing %s", f)
		} else if mode != 0 && fi.Mode().Perm() != mode {
			t.Errorf("%s mode %v", f, fi.Mode())
		}
	}
	if left, _ := filepath.Glob(filepath.Join(prefix, "bin", ".lodev*")); len(left) > 0 {
		t.Errorf("temp files left: %v", left)
	}
	for _, want := range []string{
		`export PATH="` + prefix + `/bin:$PATH"`,
		`source "` + prefix + `/share/bash-completion/completions/lodev"`,
		`fpath=("` + prefix + `/share/zsh/site-functions" $fpath)`,
		`set -p fish_complete_path "` + prefix + `/share/fish/vendor_completions.d"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestInstallScriptChecksumMismatch(t *testing.T) {
	prefix := t.TempDir()
	out, err := runInstall(t, fakeRelease(t, true), prefix)
	if err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Errorf("want a checksum failure, got %v:\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(prefix, "bin", "lodev")); !os.IsNotExist(err) {
		t.Error("lodev installed despite a bad checksum")
	}
}
