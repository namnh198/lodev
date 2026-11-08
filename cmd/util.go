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

func servicesFile() string {
	return filepath.Join(config.Home(), "services", "services.compose-base.yaml")
}
func servicesFullFile() string {
	return filepath.Join(config.Home(), "services", "services.compose-full.yaml")
}

// dockerRun is docker.Run, swappable in tests.
var dockerRun = docker.Run

// dockerPipe runs docker from | docker to; swapped in tests.
var dockerPipe = docker.Pipe

// dockerStream runs docker with the given stdin/stdout; swapped in tests.
var dockerStream = docker.Stream

// servicesCompose runs on the merged file; stacks generated before custom files existed have only the base.
// servicesComposeFile is the compose file the shared services run from: the merged one, else the
// generated one; the pre-rename names count until the next `service start` writes the new ones.
// "" when there is none.
func servicesComposeFile() string {
	dir := filepath.Dir(servicesFile())
	for _, f := range []string{servicesFullFile(), servicesFile(),
		filepath.Join(dir, "compose-full.yaml"), filepath.Join(dir, "compose.yaml")} {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	return ""
}

func servicesCompose(sub ...string) error {
	file := servicesComposeFile()
	if file == "" {
		file = servicesFile() // compose reports the missing file
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
