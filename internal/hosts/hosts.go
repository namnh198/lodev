// Package hosts manages lodev's blocks in /etc/hosts.
package hosts

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var File = "/etc/hosts"

// Render replaces (or appends) the block for name; no domains removes it.
func Render(content, name string, domains []string) string {
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
	if !found {
		out = append(out, block...)
	}
	return strings.Join(out, "\n") + "\n"
}

// Apply writes the block into File with sudo, only when it changes.
func Apply(name string, domains []string) error {
	data, err := os.ReadFile(File)
	if err != nil {
		return err
	}
	next := Render(string(data), name, domains)
	if next == string(data) {
		return nil
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
	fmt.Println("Updating", File, "(sudo)")
	for _, args := range [][]string{{"cp", File, "/etc/hosts.lodev.bak"}, {"cp", tmp.Name(), File}} {
		c := exec.Command("sudo", args...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("sudo %s: %w", strings.Join(args, " "), err)
		}
	}
	return nil
}
