package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lodev/internal/config"
	"lodev/internal/docker"
)

func servicesFile() string { return filepath.Join(config.Home(), "services", "compose.yaml") }

func servicesCompose(sub ...string) error {
	return docker.Run(docker.ComposeArgs("lodev-services", servicesFile(), sub...)...)
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
