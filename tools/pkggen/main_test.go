package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hash(c byte) string { return strings.Repeat(string(c), 64) }

func TestParseChecksums(t *testing.T) {
	in := hash('a') + "  lodev_1.2.3_linux_amd64.tar.gz\n" +
		hash('b') + "  lodev_1.2.3_darwin_arm64.tar.gz\n" +
		hash('c') + "  lodev_1.2.3_windows_amd64.zip\n"
	got, err := parseChecksums(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got["linux_amd64"] != hash('a') || got["darwin_arm64"] != hash('b') || got["windows_amd64"] != hash('c') {
		t.Fatalf("got %v", got)
	}
}

func fullData() pkgData {
	return pkgData{Version: "1.2.3", Repo: "namnh198/lodev", SHA: map[string]string{
		"darwin_amd64": hash('1'), "darwin_arm64": hash('2'), "linux_amd64": hash('3'), "linux_arm64": hash('4'),
	}}
}

func TestRender(t *testing.T) {
	out := t.TempDir()
	if err := render("../../packaging", out, fullData()); err != nil {
		t.Fatal(err)
	}
	read := func(n string) string {
		b, err := os.ReadFile(filepath.Join(out, n))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	rb := read("lodev.rb")
	for _, s := range []string{hash('1'), hash('2'), hash('3'), hash('4'),
		"releases/download/v1.2.3/lodev_1.2.3_darwin_arm64.tar.gz", `version "1.2.3"`} {
		if !strings.Contains(rb, s) {
			t.Errorf("lodev.rb lacks %q", s)
		}
	}
	pkg := read("PKGBUILD")
	for _, s := range []string{"pkgver=1.2.3", hash('3'), hash('4')} {
		if !strings.Contains(pkg, s) {
			t.Errorf("PKGBUILD lacks %q", s)
		}
	}
	if !strings.Contains(read(".SRCINFO"), "pkgver = 1.2.3") {
		t.Error(".SRCINFO lacks pkgver")
	}
}

func TestRenderMissingTarget(t *testing.T) {
	d := fullData()
	delete(d.SHA, "linux_arm64")
	err := render("../../packaging", t.TempDir(), d)
	if err == nil || !strings.Contains(err.Error(), "linux_arm64") {
		t.Fatalf("err %v", err)
	}
}
