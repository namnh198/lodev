package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"lodev/internal/config"
	"lodev/internal/docker"
)

func servicesFile() string     { return filepath.Join(config.Home(), "services", "compose.yaml") }
func servicesFullFile() string { return filepath.Join(config.Home(), "services", "compose-full.yaml") }

// dockerRun is docker.Run, swappable in tests.
var dockerRun = docker.Run

// servicesCompose runs on the merged file; stacks generated before custom files existed have only the base.
func servicesCompose(sub ...string) error {
	file := servicesFullFile()
	if _, err := os.Stat(file); err != nil {
		file = servicesFile()
	}
	return dockerRun(docker.ComposeArgs("lodev-services", file, sub...)...)
}

func readLine(prompt string) string {
	fmt.Print(prompt + " ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// confirm asks y/N; yes skips the question.
func confirm(prompt string, yes bool) bool {
	if yes {
		return true
	}
	return strings.EqualFold(readLine(prompt+" [y/N]"), "y")
}

// confirmTyped requires the user to type word exactly.
func confirmTyped(prompt, word string) bool {
	return readLine(prompt) == word
}

func stdinTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// withLock runs fn while holding ~/.lodev/lock, so two lodev commands do not
// overwrite each other's registry and /etc/hosts changes.
func withLock(fn func() error) error {
	if err := os.MkdirAll(config.Home(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(config.Home(), "lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	return fn()
}
