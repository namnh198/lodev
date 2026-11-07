package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"lodev/internal/ui"
	"lodev/internal/update"
)

var (
	executable = func() (string, error) {
		e, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(e)
	}
	latestVersion   = update.Latest
	downloadRelease = update.Download
	geteuid         = os.Geteuid
	lookSudo        = func() error { _, err := exec.LookPath("sudo"); return err }
	runSudo         = func(args ...string) error {
		c := exec.Command("sudo", args...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
)

// nightlyTag is the rolling pre-release rebuilt on every push to main.
const nightlyTag = "nightly"

var tagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// completionPaths maps a downloaded completion to where install.sh / make install put it.
var completionPaths = map[string]string{
	"lodev.bash": "share/bash-completion/completions/lodev",
	"_lodev":     "share/zsh/site-functions/_lodev",
	"lodev.fish": "share/fish/vendor_completions.d/lodev.fish",
}

type upgradeTarget struct {
	src, dst string // src is relative to the download dir
	mode     os.FileMode
	sudo     bool
}

func newSelfUpgradeCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "self-upgrade",
		Short: "Upgrade lodev to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if runtime.GOOS == "windows" {
				return errors.New("self-upgrade is not supported on Windows")
			}
			exe, err := executable()
			if err != nil {
				return err
			}
			if m := update.DetectManager(exe); m.Name != "" {
				return fmt.Errorf("lodev was installed by %s: %s", m.Name, m.Command)
			}
			tag, _ := c.Flags().GetString("version")
			if tag == "" && Channel == nightlyTag {
				tag = nightlyTag
			}
			switch tag {
			case nightlyTag:
				// nightly is a moving tag: there is no version to compare, so always fetch it.
			case "":
				ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
				tag, err = latestVersion(ctx)
				cancel()
				if err != nil {
					return err
				}
				if update.Stable(Version) && !update.Newer(Version, tag) {
					fmt.Fprintf(c.OutOrStdout(), "lodev %s is already the latest version\n", Version)
					return nil
				}
			default:
				if !strings.HasPrefix(tag, "v") {
					tag = "v" + tag
				}
				if !tagRe.MatchString(tag) {
					return fmt.Errorf("--version %q is not a version like v1.2.0", tag)
				}
				if tag == Version {
					fmt.Fprintf(c.OutOrStdout(), "lodev %s is already installed\n", tag)
					return nil
				}
			}

			// Completions first, the binary last: a failure leaves the old lodev working.
			var targets []upgradeTarget
			prefix := filepath.Dir(filepath.Dir(exe))
			for _, name := range []string{"lodev.bash", "_lodev", "lodev.fish"} {
				// Resolved like exe, so a symlinked completion (e.g. GNU stow) stays a symlink.
				if dst, err := filepath.EvalSymlinks(filepath.Join(prefix, completionPaths[name])); err == nil {
					targets = append(targets, upgradeTarget{src: filepath.Join("completions", name), dst: dst, mode: 0o644})
				}
			}
			targets = append(targets, upgradeTarget{src: "lodev", dst: exe, mode: 0o755})
			for i := range targets {
				dir := filepath.Dir(targets[i].dst)
				if targets[i].sudo = !writable(dir) && geteuid() != 0; targets[i].sudo && lookSudo() != nil {
					return fmt.Errorf("%s is not writable and sudo is missing: run lodev self-upgrade as root", dir)
				}
			}

			dl, err := downloadRelease(c.Context(), tag, runtime.GOOS, runtime.GOARCH)
			if err != nil {
				return err
			}
			defer os.RemoveAll(dl)
			for _, t := range targets {
				if err := replaceFile(filepath.Join(dl, t.src), t.dst, t.mode, t.sudo); err != nil {
					return err
				}
			}
			fmt.Fprintf(c.OutOrStdout(), "Upgraded lodev %s → %s\n", lodevVersion(), tag)
			return nil
		},
	}
	c.Flags().String("version", "", "install this version instead of the latest, e.g. v1.2.0, or nightly (built from main)")
	return c
}

// writable reports whether a file can be created in dir (mode bits alone miss ACLs and read-only mounts).
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".lodev-upgrade-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}

// replaceFile swaps dst for a copy of src by rename, never by writing into dst: rewriting a
// running, code-signed binary in place can get it killed on macOS.
func replaceFile(src, dst string, mode os.FileMode, sudo bool) error {
	dir := filepath.Dir(dst)
	if sudo {
		ui.Info("Installing %s (sudo)", dst)
		tmp := filepath.Join(dir, ".lodev-upgrade-new")
		for _, args := range [][]string{{"install", "-m", fmt.Sprintf("%04o", mode), src, tmp}, {"mv", "-f", tmp, dst}} {
			if err := runSudo(args...); err != nil {
				_ = runSudo("rm", "-f", tmp)
				return fmt.Errorf("sudo %s: %w", strings.Join(args, " "), err)
			}
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, ".lodev-upgrade-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func init() {
	rootCmd.AddCommand(newSelfUpgradeCmd())
}
