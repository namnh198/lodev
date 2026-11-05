package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lodev/internal/ui"
)

// applyFixture points File at a temp hosts file and LODEV_HOME at a temp dir, and records sudo calls.
func applyFixture(t *testing.T, content string) (home string, sudo *[][]string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("LODEV_HOME", home)
	f := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := &[][]string{}
	oldFile, oldSudo := File, runSudo
	t.Cleanup(func() { File, runSudo = oldFile, oldSudo })
	File = f
	runSudo = func(args ...string) error { *calls = append(*calls, args); return nil }
	return home, calls
}

func readHosts(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(File)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyWritableNoSudo(t *testing.T) {
	home, sudo := applyFixture(t, base)
	out, _, restore := ui.Capture(false)
	defer restore()
	if err := Apply("shop", []string{"a.test", "b.test"}); err != nil {
		t.Fatal(err)
	}
	if got := readHosts(t); got != base+shopBlock {
		t.Errorf("hosts = %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "hosts.bak")); string(b) != base {
		t.Errorf("backup = %q", b)
	}
	if len(*sudo) > 0 {
		t.Errorf("sudo used: %v", *sudo)
	}
	if s := out.String(); !strings.Contains(s, "Updating "+File) || strings.Contains(s, "(sudo)") {
		t.Errorf("output %q", s)
	}
}

func TestApplyReadOnlyUsesSudo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only file")
	}
	_, sudo := applyFixture(t, base)
	if err := os.Chmod(File, 0o444); err != nil {
		t.Fatal(err)
	}
	_, _, restore := ui.Capture(false)
	defer restore()
	if err := Apply("shop", []string{"a.test", "b.test"}); err != nil {
		t.Fatal(err)
	}
	if len(*sudo) != 2 || strings.Join((*sudo)[0], " ") != "cp "+File+" /etc/hosts.lodev.bak" ||
		(*sudo)[1][0] != "cp" || (*sudo)[1][2] != File {
		t.Errorf("sudo calls %q", *sudo)
	}
	if got := readHosts(t); got != base {
		t.Errorf("hosts changed without sudo: %q", got)
	}
}

func TestApplyUnchangedNoWrite(t *testing.T) {
	home, sudo := applyFixture(t, base+shopBlock)
	if err := Apply("shop", []string{"a.test", "b.test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "hosts.bak")); err == nil || len(*sudo) > 0 {
		t.Errorf("wrote for an unchanged block: sudo %v", *sudo)
	}
}

func TestApplyBackupFails(t *testing.T) {
	applyFixture(t, base)
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0o644)
	t.Setenv("LODEV_HOME", filepath.Join(blocker, "home"))
	_, _, restore := ui.Capture(false)
	defer restore()
	if err := Apply("shop", []string{"a.test"}); err == nil {
		t.Fatal("want an error when the backup cannot be written")
	}
	if got := readHosts(t); got != base {
		t.Errorf("hosts written without a backup: %q", got)
	}
}
