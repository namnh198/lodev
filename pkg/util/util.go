package util

import (
	"bytes"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// IsEnvTrue returns true if the given environment variable
// has a value accepted by strconv.ParseBool.
func IsEnvTrue(envVar string) bool {
	val, _ := strconv.ParseBool(os.Getenv(envVar))
	return val
}

// IsEnvFalse returns the opposite of IsEnvTrue
func IsEnvFalse(envVar string) bool {
	return !IsEnvTrue(envVar)
}

// IsInteractive checks if the current environment is interactive.
func IsInteractive() bool {
	return IsEnvTrue("LODEV_INTERACTIVE") || (IsEnvFalse("LODEV_NONINTERACTIVE"))
}

// IsVerbose checks if verbose output is enabled.
func IsVerbose() bool {
	return IsEnvTrue("LODEV_VERBOSE")
}

// Chmod changes the file permissions of the named path,
// if the path already has the necessary permissions, do nothing.
// don't use os.Chmod in the LODEV code, use util.Chmod instead.
func Chmod(path string, mode os.FileMode) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	// If the mode is the same, do nothing
	if fileInfo.Mode().Perm() == mode {
		return nil
	}
	return os.Chmod(path, mode)
}

// ParseBoolFlag scans os.Args backward to apply last-occurrence precedence for a boolean flag.
// Handles both --long[=true|false] and -s[=true|false] forms.
// Treats short flag in combined group (e.g. -xj) as implicit true.
// Returns false if the flag is absent or its value is invalid.
// Disabled entirely when running under `go test`.
func ParseBoolFlag(long string, short string) bool {
	if testing.Testing() {
		return false
	}
	args := os.Args[1:]
	longPrefix := "--" + long + "="
	shortPrefix := "-" + short + "="

	for i := len(args) - 1; i >= 0; i-- {
		arg := args[i]
		switch {
		case arg == "--"+long, arg == "-"+short:
			return true
		case strings.HasPrefix(arg, shortPrefix):
			v, err := strconv.ParseBool(arg[len(shortPrefix):])
			if err == nil {
				return v
			}
		case strings.HasPrefix(arg, longPrefix):
			v, err := strconv.ParseBool(arg[len(longPrefix):])
			if err == nil {
				return v
			}
		default:
			if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' {
				for _, ch := range arg[1:] {
					if string(ch) == short {
						return true
					}
				}
			}
		}
	}
	return false
}

// SliceToUniqueSlice processes a slice of string to make sure there are no duplicates
func SliceToUniqueSlice(inSlice *[]string) []string {
	mapStore := map[string]bool{}
	newSlice := []string{}

	for _, s := range *inSlice {
		// If we already found the value in our map, don't process into newSlice
		if _, ok := mapStore[s]; ok {
			continue
		}
		newSlice = append(newSlice, s)
		mapStore[s] = true
	}
	if len(newSlice) == 0 {
		return nil
	}
	return newSlice
}

// EnvToUniqueEnv makes sure that only the last occurrence of an env (NAME=val or bare NAME)
// slice is actually retained. Bare variable names without a value (e.g. "MY_VAR") are passed
// through as-is; docker-compose resolves them from the host environment at container start time.
func EnvToUniqueEnv(inSlice *[]string) []string {
	mapStore := map[string]string{}

	for _, s := range *inSlice {
		// Both "KEY=value" and bare "KEY" are supported.
		// strings.Cut returns the part before "=" as the key in both cases.
		// Last entry for a given key wins.
		k, _, _ := strings.Cut(s, "=")
		mapStore[k] = s
	}
	newSlice := make([]string, 0, len(mapStore))
	for _, v := range mapStore {
		newSlice = append(newSlice, v)
	}
	if len(newSlice) == 0 {
		return nil
	}
	return newSlice
}

// EscapeDollarSign the same thing is done in `docker-compose config`
// See https://github.com/docker/compose/blob/361c0893a9e16d54f535cdb2e764362363d40702/cmd/compose/config.go#L405-L409
func EscapeDollarSign(marshal []byte) []byte {
	dollar := []byte{'$'}
	escDollar := []byte{'$', '$'}
	return bytes.ReplaceAll(marshal, dollar, escDollar)
}

// IsLetter returns true if all chars in string are alpha
func IsLetter(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// IsPortValid checks if the given port string is valid (e.g. "80", "8080:80")
func IsPortValid(port string) bool {
	if !regexp.MustCompile(`^[0-9]+(:[0-9]+)?$`).MatchString(port) {
		return false
	}
	return true
}

// GetLocalIPs returns IP addresses associated with the machine
func GetLocalIPs() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	var localIPs []string
	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			if v.IP.IsLoopback() || v.IP.To4() == nil {
				continue
			}
			localIPs = append(localIPs, v.IP.String())
		case *net.IPAddr:
			if v.IP.IsLoopback() || v.IP.To4() == nil {
				continue
			}
			localIPs = append(localIPs, v.IP.String())
		}
	}

	return localIPs, nil
}

var letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandString returns a random string of given length n.
func RandString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// FindBashPath returns the PATH to Bash on any system
// on Windows preferring git-bash
// On Windows we'll need the path to Bash to execute anything.
// Returns empty string if not found, path if found
func FindBashPath() string {
	if !IsWindows() {
		return "bash"
	}

	// Check for user-local Git Bash installation first (installed for current user only)
	// This takes precedence over system-wide installations
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		userLocalBashPath := filepath.Join(localAppData, `Programs\Git\bin\bash.exe`)
		if _, err := os.Stat(userLocalBashPath); err == nil {
			return userLocalBashPath
		}
	}

	// Check for system-wide Git Bash installation using PROGRAMFILES environment variable
	// This works even if Program Files is on a different drive
	if programFiles := os.Getenv("PROGRAMFILES"); programFiles != "" {
		systemWideBashPath := filepath.Join(programFiles, `Git\bin\bash.exe`)
		if _, err := os.Stat(systemWideBashPath); err == nil {
			return systemWideBashPath
		}
	}

	// Not found - don't search PATH as it may return WSL bash which won't work
	Warning("Git Bash is not installed in standard locations, so some features like custom commands may not work correctly")
	return ""
}
