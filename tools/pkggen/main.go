// Command pkggen renders the Homebrew formula and AUR files with the release checksums.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

type pkgData struct {
	Version string            // without the leading v
	Repo    string            // owner/name on GitHub
	SHA     map[string]string // "<os>_<arch>" → sha256 of its archive
}

// outputs maps each template (relative to the templates dir) to its file name.
var outputs = map[string]string{
	"homebrew/lodev.rb.tmpl": "lodev.rb",
	"aur/PKGBUILD.tmpl":      "PKGBUILD",
	"aur/.SRCINFO.tmpl":      ".SRCINFO",
}

var archiveRe = regexp.MustCompile(`^lodev_.+_([a-z]+_[a-z0-9]+)\.(tar\.gz|zip)$`)

// parseChecksums reads sha256sum lines ("<hash>  <archive>") into "<os>_<arch>" → hash.
func parseChecksums(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		if m := archiveRe.FindStringSubmatch(f[1]); m != nil {
			out[m[1]] = f[0]
		}
	}
	return out, sc.Err()
}

// render writes every output; a file whose template fails (e.g. a missing checksum) is not written.
func render(tmplDir, outDir string, d pkgData) error {
	funcs := template.FuncMap{
		"sha": func(target string) (string, error) {
			if h := d.SHA[target]; h != "" {
				return h, nil
			}
			return "", fmt.Errorf("no checksum for %s in checksums.txt", target)
		},
		// pacman's pkgver may not contain '-'.
		"pkgver": func(v string) string { return strings.ReplaceAll(v, "-", "_") },
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for src, dst := range outputs {
		t, err := template.New(filepath.Base(src)).Funcs(funcs).ParseFiles(filepath.Join(tmplDir, src))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, d); err != nil {
			return fmt.Errorf("%s: %w", src, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, dst), buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	version := flag.String("version", "", "release version, without a leading v")
	repo := flag.String("repo", "namnh198/lodev", "GitHub owner/name")
	checksums := flag.String("checksums", "dist/checksums.txt", "sha256sum output for the release archives")
	tmpl := flag.String("templates", "packaging", "templates directory")
	out := flag.String("out", "dist/packaging", "output directory")
	flag.Parse()
	if *version == "" {
		flag.Usage()
		os.Exit(2)
	}
	f, err := os.Open(*checksums)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pkggen:", err)
		os.Exit(1)
	}
	sha, err := parseChecksums(f)
	f.Close()
	if err == nil {
		err = render(*tmpl, *out, pkgData{Version: *version, Repo: *repo, SHA: sha})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pkggen:", err)
		os.Exit(1)
	}
}
