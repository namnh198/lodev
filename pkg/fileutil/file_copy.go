package fileutil

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"text/template"

	"github.com/namnh198/lodev/pkg/nodeps"
	"github.com/namnh198/lodev/pkg/util"
)

// CopyFile copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all its contents will be replaced by the contents
// of the source file. The file mode will be copied from the source and
// the copied data is synced/flushed to stable storage. Credit @m4ng0squ4sh https://gist.github.com/m4ng0squ4sh/92462b38df26839a3ca324697c8cba04
func CopyFile(src string, dst string) error {
	in, err := os.Open(src)

	if err != nil {
		return err
	}

	defer util.CheckClose(in)

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create file %v, err: %v", src, err)
	}
	defer util.CheckClose(out)
	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("failed to copy file from %v to %v err: %v", src, dst, err)
	}

	err = out.Sync()
	if err != nil {
		return err
	}

	// os.Chmod fails on long path (> 256 characters) on windows.
	// A description of this problem with golang is at https://github.com/golang/dep/issues/774#issuecomment-311560825
	// It could end up fixed in a future version of golang.
	if !util.IsWindows() {
		si, err := os.Stat(src)
		if err != nil {
			return err
		}

		err = util.Chmod(dst, si.Mode())
		if err != nil {
			return fmt.Errorf("failed to chmod file %v to mode %v, err=%v", dst, si.Mode(), err)
		}
	}

	return nil
}

// CopyDir recursively copies a directory tree, attempting to preserve permissions.
// Source directory must exist, destination directory must *not* exist.
// Symlinks are ignored and skipped. Credit @r0l1 https://gist.github.com/r0l1/92462b38df26839a3ca324697c8cba04
func CopyDir(src string, dst string, force ...bool) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return fmt.Errorf("CopyDir: source directory %s is not a directory", src)
	}

	_, err = os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	forceCopy := len(force) > 0 && force[0]

	if !forceCopy && err == nil {
		return fmt.Errorf("CopyDir: destination %s already exists", dst)
	}

	err = os.MkdirAll(dst, si.Mode())
	if err != nil {
		return err
	}

	dirEntrySlice, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, de := range dirEntrySlice {
		srcPath := filepath.Join(src, de.Name())
		dstPath := filepath.Join(dst, de.Name())

		if de.IsDir() {
			err = CopyDir(srcPath, dstPath, force...)
			if err != nil {
				return err
			}
		} else {
			deInfo, err := de.Info()
			if err != nil {
				return err
			}
			if forceCopy && FileExists(dstPath) {
				err = os.Remove(dstPath)
				if err != nil {
					return err
				}
			}
			err = CopyFile(srcPath, dstPath)
			if err != nil && deInfo.Mode()&os.ModeSymlink != 0 {
				util.Warning("Failed to copy symlink %s, skipping...\n", srcPath)
				continue
			}
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyFilesMatchingGlob copies files matching a glob pattern from srcDir to destDir.
// It returns the list of filenames (not full paths) that were copied.
// Directories are skipped. If no files match, it returns an empty slice with no error.
func CopyFilesMatchingGlob(srcDir string, destDir string, globPattern string) ([]string, error) {
	pattern := filepath.Join(srcDir, globPattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("error finding files matching %s: %v", pattern, err)
	}

	if len(matches) == 0 {
		util.Warning("No matching glob file: %s", pattern)
	}

	var copiedFiles []string
	for _, srcFile := range matches {
		info, err := os.Stat(srcFile)
		if err != nil {
			return copiedFiles, fmt.Errorf("error stating file %s: %v", srcFile, err)
		}
		// Skip directories
		if info.IsDir() {
			continue
		}

		filename := filepath.Base(srcFile)
		destFile := filepath.Join(destDir, filename)
		if err := CopyFile(srcFile, destFile); err != nil {
			return copiedFiles, fmt.Errorf("error copying %s to %s: %v", srcFile, destFile, err)
		}
		copiedFiles = append(copiedFiles, filename)
	}

	return copiedFiles, nil
}

// CopyEmbedAssets copies files in the named embed.FS sourceDir to the local targetDir (full path)
// Some files may be excluded if they are in the excludedFiles list and contain #lodev-generated.
func CopyEmbedAssets(fsys embed.FS, sourceDir string, targetDir string, excludedFiles []string) error {
	subdirs, err := fsys.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, d := range subdirs {
		sourcePath := path.Join(sourceDir, d.Name())
		if d.IsDir() {
			err = CopyEmbedAssets(fsys, path.Join(sourceDir, d.Name()), path.Join(targetDir, d.Name()), excludedFiles)
			if err != nil {
				return err
			}
		} else {
			localPath := filepath.Join(targetDir, d.Name())

			// We can overwrite the file if it has the #lodev-generated
			// or if it is an empty file.
			sigFound, err := FGrepStringInFile(localPath, nodeps.LodevFileSignature)
			s, _ := os.Stat(localPath)
			if sigFound || (s != nil && s.Size() == 0) || err != nil {
				content, err := fsys.ReadFile(sourcePath)
				if err != nil {
					return err
				}
				err = os.MkdirAll(filepath.Dir(localPath), 0755)
				if err != nil {
					return err
				}
				if sigFound {
					// If the file already exists and has the same content, don't overwrite it.
					if existingContent, err := os.ReadFile(localPath); err == nil {
						if bytes.Equal(content, existingContent) {
							continue
						}
					}
					// If the file already exists and is excluded, don't overwrite it.
					if excludedFiles != nil && slices.Contains(excludedFiles, localPath) {
						continue
					}
				}
				err = os.WriteFile(localPath, content, 0755)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// TemplateStringToFile takes a template string, runs templ.Execute on it, and writes it out to file
func TemplateStringToFile(content string, vars map[string]any, targetFilePath string) error {
	templ := template.New("templateStringToFile:" + targetFilePath)
	templ, err := templ.Parse(content)
	if err != nil {
		return err
	}

	var doc bytes.Buffer
	err = templ.Execute(&doc, vars)
	if err != nil {
		return err
	}

	f, err := os.Create(targetFilePath)
	if err != nil {
		return err
	}
	defer util.CheckClose(f)

	_, err = f.WriteString(doc.String())
	if err != nil {
		return err
	}
	return nil
}
