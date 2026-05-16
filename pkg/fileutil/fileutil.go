package fileutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/namnh198/lodev/pkg/util"
)

// IsDirectory returns true if path is a dir, false on error or not directory
func IsDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

// IsReadable checks file has read permissions (0666)
func IsReadable(path ...string) bool {
	fullPath := filepath.Join(path...)
	file, err := os.OpenFile(fullPath, os.O_RDONLY, 0666)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

// PurgeDirectory removes all of the contents of a given
// directory, leaving the directory itself intact.
func PurgeDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}

	defer util.CheckClose(dir)

	files, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, file := range files {
		fullPath := filepath.Join(path, file)
		fi, err := os.Lstat(fullPath)
		if err != nil {
			return err
		}
		// Symlinks must be removed directly without chmod (following a symlink
		// to chmod it may fail if the target no longer exists).
		if fi.Mode()&os.ModeSymlink == 0 {
			err = util.Chmod(fullPath, 0777)
			if err != nil {
				return err
			}
		}
		err = os.RemoveAll(fullPath)
		if err != nil {
			// On Traditional Windows tests tests fail cleaning up:
			// config\default_config.yaml: The process cannot access the file because it is being used by another process
			util.Warning("unable to fully purge '%s': %v", fullPath, err)
		}
	}
	return nil
}

// PurgeDirectoryExcept removes all contents of a directory except
// files whose names are in the except map.
func PurgeDirectoryExcept(path string, except map[string]bool) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}

	defer util.CheckClose(dir)

	files, err := dir.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, file := range files {
		if except[file] {
			continue
		}
		fullPath := filepath.Join(path, file)
		fi, err := os.Lstat(fullPath)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			err = util.Chmod(fullPath, 0777)
			if err != nil {
				return err
			}
		}
		err = os.RemoveAll(fullPath)
		if err != nil {
			util.Warning("unable to remove '%s': %v", fullPath, err)
		}
	}
	return nil
}

// IsSameFile determines whether two paths refer to the same file/dir
func IsSameFile(path1 string, path2 string) (bool, error) {
	path1fi, err := os.Stat(path1)
	if err != nil {
		return false, err
	}
	path2fi, err := os.Stat(path2)
	if err != nil {
		return false, err
	}
	return os.SameFile(path1fi, path2fi), nil
}

// FgrepStringInFile is a small hammer for looking for a literal string in a file.
// It should only be used against very modest sized files, as the entire file is read into a string.
func FGrepStringInFile(fullPath string, needle string) (bool, error) {
	fullPathBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return false, fmt.Errorf("failed to open file %s, err:%v ", fullPath, err)
	}

	fullFileString := string(fullPathBytes)
	return strings.Contains(fullFileString, needle), nil
}

// GrepStringInFile is a small hammer for looking for a regex in a file.
// It should only be used against very modest sized files, as the entire file is read into a string. Returns found, matches, error
func GrepStringInFile(fullPath string, needle string) (bool, []string, error) {
	fullFileBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return false, nil, fmt.Errorf("failed to open file %s, err:%v ", fullPath, err)
	}
	fullFileString := string(fullFileBytes)
	re := regexp.MustCompile(needle)
	matches := re.FindStringSubmatch(fullFileString)
	return len(matches) > 0, matches, nil
}

// ReadFileIntoString gets the contents of file into string
func ReadFileIntoString(path string) (string, error) {
	bytesFile, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytesFile), err
}

// ListFilesInDir returns an array of files or directories found in a directory
func ListFilesInDir(path string) ([]string, error) {
	var fileList []string
	dirEntrySlice, err := os.ReadDir(path)
	if err != nil {
		return fileList, err
	}

	for _, de := range dirEntrySlice {
		fileList = append(fileList, de.Name())
	}
	return fileList, nil
}

// ListFilesInDirFullPath returns an array of full path of files found in a directory. If excludeDirectories is set, it skips subdirectories.
func ListFilesInDirFullPath(path string, excludeDirectories bool) ([]string, error) {
	var fileList []string
	dirEntrySlice, err := os.ReadDir(path)
	if err != nil {
		return fileList, err
	}

	for _, de := range dirEntrySlice {
		if excludeDirectories && de.IsDir() {
			continue
		}
		fileList = append(fileList, filepath.Join(path, de.Name()))
	}
	return fileList, nil
}

// ListFilesWithDepth returns an array of full path of files found in a directory, traversing up to maxDepth levels.
// maxDepth=0 means only files directly in dir, maxDepth=1 means dir and one level of subdirectories, etc.
// maxDepth=-1 means unlimited depth (all files recursively).
func ListFilesWithDepth(dir string, maxDepth int) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if maxDepth == -1 {
				return nil
			}
			// Calculate depth and skip directories beyond maxDepth
			relPath, _ := filepath.Rel(dir, path)
			var depth int
			if relPath == "." {
				depth = 0
			} else {
				depth = strings.Count(relPath, string(filepath.Separator)) + 1
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

// RandomFilenameBase generates a temporary filename for use in testing or whatever.
// From https://stackoverflow.com/a/28005931/215713
func RandomFilenameBase() string {
	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	return hex.EncodeToString(randBytes)
}

// CheckSignatureOrNoFile checks to make sure that a file or directory either doesn't exist
// or has #lodev-generated in its contents (so it can be overwritten)
// returns nil if overwrite is OK (if sig found or no file existing)
func CheckSignatureOrNoFile(path string, signature string) error {
	var err error
	switch {
	case !FileExists(path):
		return nil

	case FileExists(path) && !IsDirectory(path):
		found, err := FGrepStringInFile(path, signature)
		// It's unlikely that we'll get an error, but report it if we do.
		if err != nil {
			return err
		}
		// We found the file and it has the signature in it.
		if found {
			return nil
		}
		// Signature not found - check if file is empty, which means it can be safely overwritten
		s, statErr := os.Stat(path)
		if statErr == nil && s != nil && s.Size() == 0 {
			return nil // Empty file, safe to overwrite
		}
		return fmt.Errorf("signature was not found in file %s", path)

	case IsDirectory(path):
		err = filepath.WalkDir(path, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// If a directory, nothing to do, continue traversing
			if info.IsDir() {
				return nil
			}
			// If file doesn't exist, nothing to do, continue traversing
			if !FileExists(path) {
				return nil
			}
			// Now check to see if file has signature.
			found, err := FGrepStringInFile(path, signature)
			// It's unlikely that we'll get an error, but report it if we do.
			if err != nil {
				return err
			}
			// If signature found, file can be overwritten
			if found {
				return nil
			}
			// Signature not found - check if file is empty, which means it can be safely overwritten
			s, statErr := os.Stat(path)
			if statErr == nil && s != nil && s.Size() == 0 {
				return nil // Empty file, safe to overwrite
			}
			// We have the file and it does not have the signature in it and is not empty.
			// that means it's not safe to overwrite it.
			return fmt.Errorf("signature was not found in file %s", path)
		})
	}
	return err
}

// FindFilesInDirectory takes a list of files/directories and expands it into a
// a list of files only
// environment variables in list are expanded
func ExpandFilesAndDirectories(dir string, paths []string) ([]string, error) {
	var expanded []string
	origPwd, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(origPwd)
	}()
	err := os.Chdir(dir)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		path = os.ExpandEnv(path)
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					expanded = append(expanded, path)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			expanded = append(expanded, path)
		}
	}
	return expanded, nil
}

// WindowsPathToCygwinPath changes C:/path/to/something to //c/path/to/something
// This should only be used in CYGWIN/git-bash context
// Sadly, if we have a Windows drive name, it has to be converted from C:/ to //c for Win10Home/Docker toolbox
func WindowsPathToCygwinPath(windowsPath string) string {
	windowsPath = filepath.ToSlash(windowsPath)
	if len(windowsPath) >= 2 && string(windowsPath[1]) == ":" {
		drive := strings.ToLower(string(windowsPath[0]))
		windowsPath = "/" + drive + windowsPath[2:]
	}
	return windowsPath
}

// ShortHomeJoin returns the same result as filepath.Join() path with $HOME/ replaced by ~/
func ShortHomeJoin(path ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		util.Failed("Could not get home directory for current user. Is it set? err=%v", err)
	}
	homeSlash := filepath.ToSlash(home)
	joined := filepath.ToSlash(filepath.Join(path...))
	if strings.HasPrefix(joined, homeSlash) {
		return "~" + joined[len(homeSlash):]
	}
	return joined
}
