package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"lodev/internal/config"
	"lodev/internal/ui"
	"lodev/internal/update"
)

var (
	stderrIsTerminal = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }
	now              = time.Now
	updateTimeout    = 3 * time.Second
	// updateResult gets the latest tag ("" if unknown) of the check this run started.
	updateResult chan string
)

// startUpdateCheck looks up the latest release in the background, from the cache when it is fresh.
func startUpdateCheck(c *cobra.Command) {
	updateResult = nil
	if !updateCheckApplies(c) {
		return
	}
	ch := make(chan string, 1)
	updateResult = ch
	go func() { ch <- latestCached() }()
}

func updateCheckApplies(c *cobra.Command) bool {
	if !update.Stable(Version) || runtime.GOOS == "windows" || !stderrIsTerminal() {
		return false
	}
	if slices.Contains([]string{"self-upgrade", "completion", "help", "__complete", "__completeNoDesc"}, c.Name()) ||
		c.HasParent() && c.Parent().Name() == "completion" {
		return false
	}
	g, err := config.LoadGlobal()
	return err == nil && g.UpdateCheck
}

func latestCached() string {
	p := filepath.Join(config.Home(), "update-check.json")
	cache := update.LoadCache(p)
	if cache.Fresh(now()) {
		return cache.Latest
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	if tag, err := latestVersion(ctx); err != nil {
		ui.Debug("update check: %v", err)
	} else {
		cache.Latest = tag
	}
	// Saved on failure too, so an offline machine does not retry on every command.
	cache.CheckedAt = now()
	if err := update.SaveCache(p, cache); err != nil {
		ui.Debug("update check: %v", err)
	}
	return cache.Latest
}

// finishUpdateCheck waits up to wait for the check and tells the user about a newer release.
func finishUpdateCheck(wait time.Duration) {
	ch := updateResult
	if ch == nil {
		return
	}
	updateResult = nil
	select {
	case latest := <-ch:
		if !update.Newer(Version, latest) {
			return
		}
		how := "lodev self-upgrade"
		if exe, err := executable(); err == nil {
			how = update.DetectManager(exe).Command
		}
		ui.Warn("lodev %s is available (you have %s). Run: %s", latest, Version, how)
	case <-time.After(wait):
	}
}
