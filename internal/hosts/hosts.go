// Package hosts manages lodev's blocks in /etc/hosts.
package hosts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"lodev/internal/config"
	"lodev/internal/ui"
)

var File = "/etc/hosts"

// runSudo runs sudo with the terminal attached, so it can ask for the password; swapped in tests.
var runSudo = func(args ...string) error {
	c := exec.Command("sudo", args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Render replaces (or appends) the block for name; no domains removes it.
func Render(content, name string, domains []string) (string, error) {
	begin, end := "# lodev:"+name+" begin", "# lodev:"+name+" end"
	var block []string
	if len(domains) > 0 {
		block = []string{begin, "127.0.0.1 " + strings.Join(domains, " "), end}
	}
	var out []string
	found, inBlock := false, false
	for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		switch {
		case line == begin:
			found, inBlock = true, true
			out = append(out, block...)
		case line == end:
			inBlock = false
		case !inBlock:
			out = append(out, line)
		}
	}
	if inBlock {
		return "", fmt.Errorf("%s has %q without %q; fix the file by hand", File, begin, end)
	}
	if !found {
		out = append(out, block...)
	}
	return strings.Join(out, "\n") + "\n", nil
}

// Block is one lodev block of File: a name and its domains (none removes the block).
type Block struct {
	Name    string
	Domains []string
}

// Apply writes the block into File, only when it changes: directly when File is writable, else with sudo.
func Apply(name string, domains []string) error { return ApplyAll([]Block{{name, domains}}) }

// ApplyAll writes several blocks with a single write (one sudo prompt, one backup).
func ApplyAll(blocks []Block) error {
	data, err := os.ReadFile(File)
	if err != nil {
		return err
	}
	next := string(data)
	for _, b := range blocks {
		if next, err = Render(next, b.Name, b.Domains); err != nil {
			return err
		}
	}
	if next == string(data) {
		return nil
	}
	// No O_TRUNC: the probe must not change the file.
	if f, err := os.OpenFile(File, os.O_WRONLY, 0); err == nil {
		f.Close()
		// The backup lives in lodev's home: a non-root user cannot create files in /etc.
		if err := os.MkdirAll(config.Home(), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(config.Home(), "hosts.bak"), data, 0o644); err != nil {
			return err
		}
		ui.Info("Updating %s", File)
		// In place: /etc may not allow the rename an atomic write needs.
		return os.WriteFile(File, []byte(next), 0o644)
	}
	tmp, err := os.CreateTemp("", "lodev-hosts-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(next); err != nil {
		return err
	}
	tmp.Close()
	ui.Info("Updating %s (sudo)", File)
	for _, args := range [][]string{{"cp", File, "/etc/hosts.lodev.bak"}, {"cp", tmp.Name(), File}} {
		if err := runSudo(args...); err != nil {
			return fmt.Errorf("sudo %s: %w", strings.Join(args, " "), err)
		}
	}
	return nil
}

// Domains is every domain inside lodev's blocks of content (the text of File).
func Domains(content string) map[string]bool {
	out := map[string]bool{}
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "# lodev:") && strings.HasSuffix(line, " begin"):
			inBlock = true
		case strings.HasPrefix(line, "# lodev:") && strings.HasSuffix(line, " end"):
			inBlock = false
		case inBlock:
			if f := strings.Fields(line); len(f) > 1 {
				for _, d := range f[1:] {
					out[d] = true
				}
			}
		}
	}
	return out
}
