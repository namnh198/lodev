// Package assets holds files embedded into the lodev binary.
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:php all:mkcert all:templates
var FS embed.FS

// Extract copies the embedded subtree sub into dst.
func Extract(sub, dst string) error {
	root, err := fs.Sub(FS, sub)
	if err != nil {
		return err
	}
	return fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
}

// Hash fingerprints the embedded subtree sub, so a changed asset can be detected in a built image.
func Hash(sub string) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(FS, sub, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(FS, path)
		h.Write([]byte(path + "\x00"))
		h.Write(data)
		return err
	})
	return hex.EncodeToString(h.Sum(nil)), err
}
