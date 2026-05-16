package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/namnh198/lodev/pkg/util"
)

type XSymContents struct {
	LinkLocation string
	LinkTarget   string
}

// FindSimulatedXsymSymlinks searches the basePath provided for files
// whose first line is XSym, which is used in cifs filesystem for simulated
// symlinks.
func FindSimulatedXsymSymlinks(basePath string) ([]XSymContents, error) {
	symLinks := make([]XSymContents, 0)
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// TODO: Skip a directory named .git? Skip other arbitrary dirs or files?
		if !info.IsDir() {
			if info.Size() == 1067 {
				contents, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				lines := strings.Split(string(contents), "\n")
				if lines[0] != "XSym" {
					return nil
				}
				if len(lines) < 4 {
					return fmt.Errorf("apparent XSym doesn't have enough lines: %s", path)
				}
				// target is 4th line
				linkTarget := filepath.Clean(lines[3])
				symLinks = append(symLinks, XSymContents{LinkLocation: path, LinkTarget: linkTarget})
			}
		}
		return nil
	})
	return symLinks, err
}

// ReplaceSimulatedXsymSymlinks walks a list of XSymContents and makes real symlinks
// in their place. This is only valid on Windows host, only works with Docker for Windows
// (cifs filesystem)
func ReplaceSimulatedXsymSymlinks(links []XSymContents) error {
	for _, item := range links {
		err := os.Remove(item.LinkLocation)
		if err != nil {
			return err
		}
		err = os.Symlink(item.LinkTarget, item.LinkLocation)
		if err != nil {
			return err
		}
	}
	return nil
}

// CanCreateSymlinks tests to see if it's possible to create a symlink
func CanCreateSymlinks() bool {
	tmpdir := os.TempDir()
	linkPath := filepath.Join(tmpdir, RandomFilenameBase())
	// This doesn't attempt to create the real file; we don't need it.
	err := os.Symlink(filepath.Join(tmpdir, "realfile.txt"), linkPath)
	//nolint: errcheck
	defer os.Remove(linkPath)
	if err != nil {
		return false
	}
	return true
}

// ReplaceSimulatedLinks walks the path provided and tries to replace XSym links with real ones.
func ReplaceSimulatedLinks(path string) {
	links, err := FindSimulatedXsymSymlinks(path)
	if err != nil {
		util.Warning("Error finding XSym Symlinks: %v", err)
	}
	if len(links) == 0 {
		return
	}

	if !CanCreateSymlinks() {
		util.Warning("This host computer is unable to create real symlinks, please see the docs to enable developer mode:\n%s\nNote that the simulated symlinks created inside the container will work fine for most projects.", "https://docs.lodev.com/en/stable/users/usage/developer-tools/#windows-os-and-lodev-composer")
		return
	}

	err = ReplaceSimulatedXsymSymlinks(links)
	if err != nil {
		util.Warning("Failed replacing simulated symlinks: %v", err)
	}
	replacedLinks := make([]string, 0)
	for _, l := range links {
		replacedLinks = append(replacedLinks, l.LinkLocation)
	}
	util.Success("Replaced these simulated symlinks with real symlinks: %v", replacedLinks)
}
