package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/ui"
	"lodev/internal/update"
)

var checkNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

// checkFixture: v1.2.0 on a terminal, manual install, network answers latest (or err).
func checkFixture(t *testing.T, latest string, err error) *int {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	calls := new(int)
	oldVer, oldTerm, oldNow, oldLatest, oldExe := Version, stderrIsTerminal, now, latestVersion, executable
	t.Cleanup(func() {
		Version, stderrIsTerminal, now, latestVersion, executable = oldVer, oldTerm, oldNow, oldLatest, oldExe
	})
	Version = "v1.2.0"
	stderrIsTerminal = func() bool { return true }
	now = func() time.Time { return checkNow }
	latestVersion = func(context.Context) (string, error) { *calls++; return latest, err }
	executable = func() (string, error) { return "/usr/local/bin/lodev", nil }
	return calls
}

func cachePath() string { return filepath.Join(config.Home(), "update-check.json") }

func writeCache(t *testing.T, age time.Duration, latest string) {
	t.Helper()
	if err := update.SaveCache(cachePath(), update.Cache{CheckedAt: checkNow.Add(-age), Latest: latest}); err != nil {
		t.Fatal(err)
	}
}

// runCheck runs the check around a command named name (under parent, if given); returns stderr.
func runCheck(t *testing.T, name, parent string) string {
	t.Helper()
	_, e, restore := ui.Capture(false)
	defer restore()
	c := &cobra.Command{Use: name}
	if parent != "" {
		(&cobra.Command{Use: parent}).AddCommand(c)
	}
	startUpdateCheck(c)
	finishUpdateCheck(time.Second)
	return e.String()
}

const notice = "lodev v1.3.0 is available (you have v1.2.0). Run: lodev self-upgrade"

func TestUpdateNotice(t *testing.T) {
	checkFixture(t, "v1.3.0", nil)
	mustContain(t, runCheck(t, "list", ""), notice)
	if c := update.LoadCache(cachePath()); c.Latest != "v1.3.0" || !c.CheckedAt.Equal(checkNow) {
		t.Errorf("cache %+v", c)
	}
}

func TestUpdateCheckCacheFresh(t *testing.T) {
	calls := checkFixture(t, "v9.9.9", nil)
	writeCache(t, time.Hour, "v1.3.0")
	mustContain(t, runCheck(t, "list", ""), notice)
	if *calls != 0 {
		t.Errorf("%d network calls with a fresh cache", *calls)
	}
}

func TestUpdateCheckCacheStale(t *testing.T) {
	calls := checkFixture(t, "v1.3.0", nil)
	writeCache(t, 5*time.Hour, "v1.2.0")
	mustContain(t, runCheck(t, "list", ""), notice)
	if *calls != 1 {
		t.Errorf("%d network calls", *calls)
	}
}

func TestUpdateCheckFailureCached(t *testing.T) {
	calls := checkFixture(t, "", errors.New("offline"))
	writeCache(t, 5*time.Hour, "v1.3.0")
	mustContain(t, runCheck(t, "list", ""), notice)
	if c := update.LoadCache(cachePath()); c.Latest != "v1.3.0" || !c.CheckedAt.Equal(checkNow) {
		t.Errorf("cache %+v", c)
	}
	runCheck(t, "list", "")
	if *calls != 1 {
		t.Errorf("%d network calls, want 1", *calls)
	}
}

func TestUpdateNoticeNotOlder(t *testing.T) {
	checkFixture(t, "", nil)
	Version = "v1.4.0"
	writeCache(t, time.Hour, "v1.3.0")
	if got := runCheck(t, "list", ""); got != "" {
		t.Errorf("stderr %q", got)
	}
}

func TestUpdateCheckSkips(t *testing.T) {
	for _, tt := range []struct {
		name, cmd, parent string
		setup             func(t *testing.T)
	}{
		{"dev build", "list", "", func(*testing.T) { Version = "dev" }},
		{"not a terminal", "list", "", func(*testing.T) { stderrIsTerminal = func() bool { return false } }},
		{"turned off", "list", "", func(t *testing.T) {
			if err := os.WriteFile(config.GlobalPath(), []byte("update_check: false\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"self-upgrade", "self-upgrade", "", func(*testing.T) {}},
		{"help", "help", "", func(*testing.T) {}},
		{"__complete", "__complete", "", func(*testing.T) {}},
		{"completion bash", "bash", "completion", func(*testing.T) {}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := checkFixture(t, "v1.3.0", nil)
			tt.setup(t)
			if got := runCheck(t, tt.cmd, tt.parent); got != "" {
				t.Errorf("stderr %q", got)
			}
			if _, err := os.Stat(cachePath()); *calls != 0 || !os.IsNotExist(err) {
				t.Errorf("%d calls, cache file: %v", *calls, err)
			}
		})
	}
}

func TestUpdateNoticeManager(t *testing.T) {
	checkFixture(t, "v1.3.0", nil)
	executable = func() (string, error) { return "/opt/homebrew/Cellar/lodev/1.2.0/bin/lodev", nil }
	got := runCheck(t, "list", "")
	if !strings.HasSuffix(strings.TrimSpace(got), "Run: brew upgrade lodev") {
		t.Errorf("stderr %q", got)
	}
}

func TestUpdateCheckSlow(t *testing.T) {
	checkFixture(t, "", nil)
	defer func(d time.Duration) { updateTimeout = d }(updateTimeout)
	updateTimeout = 200 * time.Millisecond
	latestVersion = func(ctx context.Context) (string, error) { <-ctx.Done(); return "", ctx.Err() }
	_, e, restore := ui.Capture(false)
	defer restore()
	startUpdateCheck(&cobra.Command{Use: "list"})
	ch := updateResult
	start := time.Now()
	finishUpdateCheck(50 * time.Millisecond)
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("finish waited %v", d)
	}
	if e.Len() != 0 {
		t.Errorf("stderr %q", e.String())
	}
	<-ch // let the abandoned check finish before LODEV_HOME and ui are restored
}

func TestUpdateCheckSlowStillCached(t *testing.T) {
	calls := checkFixture(t, "", nil)
	writeCache(t, 5*time.Hour, "v1.3.0")
	release := make(chan struct{})
	latestVersion = func(ctx context.Context) (string, error) {
		*calls++
		<-release
		return "", errors.New("hung")
	}
	_, _, restore := ui.Capture(false)
	defer restore()
	startUpdateCheck(&cobra.Command{Use: "list"})
	ch := updateResult
	finishUpdateCheck(50 * time.Millisecond)
	// The process would exit here; checked_at must already have moved.
	c := update.LoadCache(cachePath())
	close(release)
	<-ch
	if !c.CheckedAt.Equal(checkNow) || c.Latest != "v1.3.0" {
		t.Errorf("cache before the hung check ended: %+v", c)
	}
}
