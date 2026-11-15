// Package atomicfile writes files through a temp file renamed into place.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces path with data through a temp file renamed into place, so a reader
// (or Traefik's directory watch) never sees a half-written file. A symlink is written through.
func Write(path string, data []byte, mode os.FileMode) error {
	target := path
	if t, err := filepath.EvalSymlinks(path); err == nil {
		target = t
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(target)+".*.tmp") // unique: concurrent writers never share it
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, target)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
