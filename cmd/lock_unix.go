//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive lock on f, waiting until it is free.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}
