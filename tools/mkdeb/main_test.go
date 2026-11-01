package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type member struct {
	name string
	data []byte
}

// readAr parses the ar format as dpkg does: 8-byte magic, 60-byte headers, 2-byte aligned bodies.
func readAr(t *testing.T, b []byte) []member {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("!<arch>\n")) {
		t.Fatalf("no ar magic: %q", b[:8])
	}
	var out []member
	for off := 8; off < len(b); {
		h := b[off : off+60]
		if string(h[58:60]) != "`\n" {
			t.Fatalf("bad header end at %d: %q", off, h)
		}
		size, err := strconv.Atoi(strings.TrimSpace(string(h[48:58])))
		if err != nil {
			t.Fatal(err)
		}
		off += 60
		out = append(out, member{strings.TrimSpace(string(h[0:16])), b[off : off+size]})
		off += size
		if size%2 == 1 {
			if b[off] != '\n' {
				t.Fatalf("odd member %q not padded with \\n", out[len(out)-1].name)
			}
			off++
		}
	}
	return out
}

func TestWriteAr(t *testing.T) {
	var buf bytes.Buffer
	if err := writeAr(&buf, []arFile{{"a", []byte("xyz")}, {"bb", []byte("12")}}); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if second := 8 + 60 + 3 + 1; string(b[second:second+2]) != "bb" {
		t.Fatalf("second header not at %d: %q", second, b[second:second+16])
	}
	m := readAr(t, b)
	if len(m) != 2 || m[0].name != "a" || string(m[0].data) != "xyz" || m[1].name != "bb" || string(m[1].data) != "12" {
		t.Fatalf("members %+v", m)
	}
}

func untar(t *testing.T, gz []byte) map[string]*tar.Header {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	hs := map[string]*tar.Header{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return hs
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "./control" {
			b, _ := io.ReadAll(tr)
			h.PAXRecords = map[string]string{"body": string(b)}
		}
		hs[h.Name] = h
	}
}

func TestBuildDeb(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "lodev")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)
	comp := filepath.Join(dir, "completions")
	os.MkdirAll(comp, 0o755)
	for _, f := range []string{"lodev.bash", "_lodev", "lodev.fish"} {
		os.WriteFile(filepath.Join(comp, f), []byte(f), 0o644)
	}
	var buf bytes.Buffer
	if err := buildDeb("1.2.3", "arm64", bin, comp, &buf); err != nil {
		t.Fatal(err)
	}
	m := readAr(t, buf.Bytes())
	if len(m) != 3 || m[0].name != "debian-binary" || m[1].name != "control.tar.gz" || m[2].name != "data.tar.gz" {
		t.Fatalf("members %v", m)
	}
	if string(m[0].data) != "2.0\n" {
		t.Errorf("debian-binary %q", m[0].data)
	}
	control := untar(t, m[1].data)["./control"].PAXRecords["body"]
	for _, s := range []string{"Package: lodev\n", "Version: 1.2.3\n", "Architecture: arm64\n"} {
		if !strings.Contains(control, s) {
			t.Errorf("control lacks %q:\n%s", s, control)
		}
	}
	data := untar(t, m[2].data)
	want := map[string]int64{
		"./usr/bin/lodev": 0o755,
		"./usr/share/bash-completion/completions/lodev":    0o644,
		"./usr/share/zsh/vendor-completions/_lodev":        0o644,
		"./usr/share/fish/vendor_completions.d/lodev.fish": 0o644,
	}
	for name, mode := range want {
		h, ok := data[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if h.Mode != mode || h.Uid != 0 || h.Gid != 0 {
			t.Errorf("%s mode %o uid %d gid %d", name, h.Mode, h.Uid, h.Gid)
		}
	}
	if h, ok := data["./usr/bin/"]; !ok || h.Typeflag != tar.TypeDir {
		t.Errorf("parent dir ./usr/bin/ missing")
	}
}
