// Command mkdeb builds lodev's .deb without dpkg-deb, so packages can be made on macOS too.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type arFile struct {
	Name string
	Data []byte
}

// writeAr writes the common ar format dpkg reads: magic, 60-byte headers, bodies padded to even length.
func writeAr(w io.Writer, files []arFile) error {
	if _, err := io.WriteString(w, "!<arch>\n"); err != nil {
		return err
	}
	mtime := time.Now().Unix()
	for _, f := range files {
		hdr := fmt.Sprintf("%-16s%-12d%-6s%-6s%-8s%-10d`\n", f.Name, mtime, "0", "0", "100644", len(f.Data))
		if _, err := io.WriteString(w, hdr); err != nil {
			return err
		}
		if _, err := w.Write(f.Data); err != nil {
			return err
		}
		if len(f.Data)%2 == 1 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

type entry struct {
	path string // inside the archive, without the leading "./"
	mode int64
	data []byte
}

// tarGz packs entries (and their parent directories) owned by root.
func tarGz(entries []entry) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	now := time.Now()
	seen := map[string]bool{}
	for _, e := range entries {
		var dirs []string
		for d := path.Dir(e.path); d != "."; d = path.Dir(d) {
			dirs = append([]string{d}, dirs...)
		}
		for _, d := range dirs {
			if seen[d] {
				continue
			}
			seen[d] = true
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "./" + d + "/", Mode: 0o755, ModTime: now, Uname: "root", Gname: "root"}); err != nil {
				return nil, err
			}
		}
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "./" + e.path, Mode: e.mode, Size: int64(len(e.data)), ModTime: now, Uname: "root", Gname: "root"}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildDeb writes a .deb with the binary and the bash/zsh/fish completions from completions/.
func buildDeb(version, arch, bin, completions string, w io.Writer) error {
	b, err := os.ReadFile(bin)
	if err != nil {
		return err
	}
	data := []entry{{"usr/bin/lodev", 0o755, b}}
	for src, dst := range map[string]string{
		"lodev.bash": "usr/share/bash-completion/completions/lodev",
		"_lodev":     "usr/share/zsh/vendor-completions/_lodev",
		"lodev.fish": "usr/share/fish/vendor_completions.d/lodev.fish",
	} {
		c, err := os.ReadFile(filepath.Join(completions, src))
		if err != nil {
			return err
		}
		data = append(data, entry{dst, 0o644, c})
	}
	slices.SortFunc(data, func(a, b entry) int { return strings.Compare(a.path, b.path) }) // stable order: maps iterate randomly
	dataTgz, err := tarGz(data)
	if err != nil {
		return err
	}
	control := strings.Join([]string{
		"Package: lodev",
		"Version: " + version,
		"Architecture: " + arch,
		"Maintainer: namnh198 <me@namhoainguyen.com>",
		"Recommends: docker-ce | docker.io",
		"Section: devel",
		"Priority: optional",
		"Homepage: https://github.com/namnh198/lodev",
		"Description: DDEV-like local PHP development with shared Docker services",
		" lodev runs one small PHP container per project and shares MySQL/MariaDB,",
		" Redis, OpenSearch, Mailpit and phpMyAdmin across all projects.",
	}, "\n") + "\n"
	controlTgz, err := tarGz([]entry{{"control", 0o644, []byte(control)}})
	if err != nil {
		return err
	}
	return writeAr(w, []arFile{
		{"debian-binary", []byte("2.0\n")},
		{"control.tar.gz", controlTgz},
		{"data.tar.gz", dataTgz},
	})
}

func main() {
	version := flag.String("version", "", "package version, without a leading v")
	arch := flag.String("arch", "", "Debian architecture: amd64 or arm64")
	bin := flag.String("bin", "", "path to the linux lodev binary")
	completions := flag.String("completions", "", "directory with lodev.bash, _lodev, lodev.fish")
	out := flag.String("out", "", "output .deb path")
	flag.Parse()
	if *version == "" || *arch == "" || *bin == "" || *completions == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	var buf bytes.Buffer
	if err := buildDeb(*version, *arch, *bin, *completions, &buf); err != nil {
		fmt.Fprintln(os.Stderr, "mkdeb:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "mkdeb:", err)
		os.Exit(1)
	}
}
