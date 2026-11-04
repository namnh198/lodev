package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"lodev/internal/ui"
)

type upgradeFix struct {
	prefix, dl        string
	downloads, latest int
	tags              []string
	sudo              [][]string
}

// upgradeFixture is a manual install under a temp prefix, v1.2.0, with v1.3.0 released.
func upgradeFixture(t *testing.T) *upgradeFix {
	t.Helper()
	f := &upgradeFix{prefix: t.TempDir(), dl: t.TempDir()}
	for p, body := range map[string]string{
		"bin/lodev": "old",
		"share/bash-completion/completions/lodev": "c-old",
		"share/zsh/site-functions/_lodev":         "c-old",
	} {
		writeFile(t, filepath.Join(f.prefix, p), body)
	}
	writeFile(t, filepath.Join(f.dl, "lodev"), "new")
	for _, c := range []string{"lodev.bash", "_lodev", "lodev.fish"} {
		writeFile(t, filepath.Join(f.dl, "completions", c), "c-new")
	}
	oldVer, oldExe, oldLatest, oldDl, oldEuid, oldLook, oldSudo := Version, executable, latestVersion, downloadRelease, geteuid, lookSudo, runSudo
	t.Cleanup(func() {
		Version, executable, latestVersion, downloadRelease, geteuid, lookSudo, runSudo = oldVer, oldExe, oldLatest, oldDl, oldEuid, oldLook, oldSudo
	})
	Version = "v1.2.0"
	executable = func() (string, error) { return filepath.Join(f.prefix, "bin", "lodev"), nil }
	latestVersion = func(context.Context) (string, error) { f.latest++; return "v1.3.0", nil }
	downloadRelease = func(_ context.Context, tag, _, _ string) (string, error) {
		f.downloads++
		f.tags = append(f.tags, tag)
		// self-upgrade removes the dir it gets; hand it a copy.
		dir := t.TempDir()
		copyTree(t, f.dl, dir)
		return dir, nil
	}
	geteuid = func() int { return 1000 }
	lookSudo = func() error { return nil }
	runSudo = func(args ...string) error { f.sudo = append(f.sudo, args); return nil }
	return f
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, _ := os.ReadFile(p)
		writeFile(t, filepath.Join(dst, rel), string(b))
		return nil
	})
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return string(b)
}

// runUpgrade runs self-upgrade; out is the command's stdout plus ui output.
func runUpgrade(t *testing.T, args ...string) (string, error) {
	t.Helper()
	o, e, restore := ui.Capture(false)
	defer restore()
	c := newSelfUpgradeCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err := c.Execute()
	return out.String() + o.String() + e.String(), err
}

func readOnly(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only folder")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
}

func TestSelfUpgrade(t *testing.T) {
	f := upgradeFixture(t)
	out, err := runUpgrade(t)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "Upgraded lodev v1.2.0 → v1.3.0")
	bin := filepath.Join(f.prefix, "bin", "lodev")
	if got := readFile(t, bin); got != "new" {
		t.Errorf("binary = %q", got)
	}
	if fi, _ := os.Stat(bin); fi.Mode().Perm() != 0o755 {
		t.Errorf("binary mode %v", fi.Mode())
	}
	for _, p := range []string{"share/bash-completion/completions/lodev", "share/zsh/site-functions/_lodev"} {
		p = filepath.Join(f.prefix, p)
		if got := readFile(t, p); got != "c-new" {
			t.Errorf("%s = %q", p, got)
		}
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
			t.Errorf("%s mode %v", p, fi.Mode())
		}
	}
	if _, err := os.Stat(filepath.Join(f.prefix, "share/fish/vendor_completions.d/lodev.fish")); !os.IsNotExist(err) {
		t.Error("fish completion must not be created")
	}
	if left, _ := filepath.Glob(filepath.Join(f.prefix, "bin", ".lodev-upgrade-*")); len(left) > 0 {
		t.Errorf("temp files left: %v", left)
	}
	if len(f.sudo) > 0 {
		t.Errorf("sudo used for writable folders: %v", f.sudo)
	}
}

func TestSelfUpgradeAlreadyLatest(t *testing.T) {
	f := upgradeFixture(t)
	Version = "v1.3.0"
	out, err := runUpgrade(t)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "lodev v1.3.0 is already the latest version")
	if f.downloads != 0 {
		t.Errorf("%d downloads", f.downloads)
	}
}

func TestSelfUpgradeVersionFlag(t *testing.T) {
	f := upgradeFixture(t)
	if _, err := runUpgrade(t, "--version", "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.tags, []string{"v1.1.0"}) || f.latest != 0 {
		t.Errorf("tags %v, latest calls %d", f.tags, f.latest)
	}
	f.downloads = 0
	out, err := runUpgrade(t, "--version", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "lodev v1.2.0 is already installed")
	if f.downloads != 0 {
		t.Errorf("%d downloads", f.downloads)
	}
	if _, err := runUpgrade(t, "--version", "nope"); err == nil {
		t.Error("--version nope: want error")
	}
}

func TestSelfUpgradeSudo(t *testing.T) {
	f := upgradeFixture(t)
	bin := filepath.Join(f.prefix, "bin")
	readOnly(t, bin)
	out, err := runUpgrade(t)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(bin, ".lodev-upgrade-new")
	if len(f.sudo) != 2 || f.sudo[0][0] != "install" || !reflect.DeepEqual(f.sudo[0][1:3], []string{"-m", "0755"}) ||
		filepath.Base(f.sudo[0][3]) != "lodev" || f.sudo[0][4] != tmp ||
		!reflect.DeepEqual(f.sudo[1], []string{"mv", "-f", tmp, filepath.Join(bin, "lodev")}) {
		t.Errorf("sudo calls %q", f.sudo)
	}
	if got := readFile(t, filepath.Join(f.prefix, "share/zsh/site-functions/_lodev")); got != "c-new" {
		t.Errorf("zsh completion = %q", got)
	}
	mustContain(t, out, "Installing "+filepath.Join(bin, "lodev")+" (sudo)")
}

func TestSelfUpgradeSudoMissing(t *testing.T) {
	f := upgradeFixture(t)
	bin := filepath.Join(f.prefix, "bin")
	readOnly(t, bin)
	lookSudo = func() error { return errors.New("not found") }
	_, err := runUpgrade(t)
	if want := bin + " is not writable and sudo is missing: run lodev self-upgrade as root"; err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
	if f.downloads != 0 || len(f.sudo) != 0 {
		t.Errorf("%d downloads, sudo %v", f.downloads, f.sudo)
	}
}

func TestSelfUpgradeSudoFails(t *testing.T) {
	f := upgradeFixture(t)
	bin := filepath.Join(f.prefix, "bin")
	readOnly(t, bin)
	runSudo = func(args ...string) error {
		f.sudo = append(f.sudo, args)
		if args[0] == "mv" {
			return errors.New("exit status 1")
		}
		return nil
	}
	_, err := runUpgrade(t)
	if err == nil || !strings.HasPrefix(err.Error(), "sudo mv -f ") {
		t.Errorf("err = %v", err)
	}
	if last := f.sudo[len(f.sudo)-1]; !reflect.DeepEqual(last, []string{"rm", "-f", filepath.Join(bin, ".lodev-upgrade-new")}) {
		t.Errorf("last sudo call %q", last)
	}
	if got := readFile(t, filepath.Join(bin, "lodev")); got != "old" {
		t.Errorf("binary = %q", got)
	}
}

func TestSelfUpgradeManager(t *testing.T) {
	f := upgradeFixture(t)
	executable = func() (string, error) { return filepath.Join(f.prefix, "Cellar/lodev/1.2.0/bin/lodev"), nil }
	_, err := runUpgrade(t)
	if err == nil || err.Error() != "lodev was installed by Homebrew: brew upgrade lodev" {
		t.Errorf("err = %v", err)
	}
	if f.downloads != 0 {
		t.Errorf("%d downloads", f.downloads)
	}
}

func TestSelfUpgradeFollowsSymlink(t *testing.T) {
	f := upgradeFixture(t)
	link := filepath.Join(f.prefix, "bin", "lodev")
	real := filepath.Join(f.prefix, "libexec", "lodev")
	writeFile(t, real, "old")
	os.Remove(link)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	executable = func() (string, error) { return filepath.EvalSymlinks(link) }
	if _, err := runUpgrade(t); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, real); got != "new" {
		t.Errorf("real binary = %q", got)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("bin/lodev is no longer a symlink: %v %v", fi, err)
	}
}

func TestSelfUpgradeDownloadError(t *testing.T) {
	f := upgradeFixture(t)
	downloadRelease = func(context.Context, string, string, string) (string, error) {
		return "", errors.New("checksum mismatch")
	}
	if _, err := runUpgrade(t); err == nil {
		t.Error("want error")
	}
	if got := readFile(t, filepath.Join(f.prefix, "bin", "lodev")); got != "old" {
		t.Errorf("binary = %q", got)
	}
}

func TestSelfUpgradeEdgeChannel(t *testing.T) {
	f := upgradeFixture(t)
	defer func(c string) { Channel = c }(Channel)
	Version, Channel = "v0.1.0-14-g238f647", "edge"
	out, err := runUpgrade(t)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.tags, []string{"edge"}) || f.latest != 0 {
		t.Errorf("tags %v, latest calls %d", f.tags, f.latest)
	}
	mustContain(t, out, "Upgraded lodev v0.1.0-14-g238f647 → edge")
}

func TestSelfUpgradeVersionEdge(t *testing.T) {
	f := upgradeFixture(t)
	if _, err := runUpgrade(t, "--version", "edge"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.tags, []string{"edge"}) || f.latest != 0 {
		t.Errorf("tags %v, latest calls %d", f.tags, f.latest)
	}
	if got := readFile(t, filepath.Join(f.prefix, "bin", "lodev")); got != "new" {
		t.Errorf("binary = %q", got)
	}
}

func TestSelfUpgradeLatestHasDeadline(t *testing.T) {
	upgradeFixture(t)
	var hasDeadline bool
	latestVersion = func(ctx context.Context) (string, error) {
		_, hasDeadline = ctx.Deadline()
		return "v1.2.0", nil
	}
	if _, err := runUpgrade(t); err != nil {
		t.Fatal(err)
	}
	if !hasDeadline {
		t.Error("the latest-version lookup has no deadline")
	}
}
