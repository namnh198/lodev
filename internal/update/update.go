// Package update finds, compares and downloads lodev releases.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LatestURL redirects to the newest stable release's tag page.
var LatestURL = "https://github.com/namnh198/lodev/releases/latest"

var stableRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// Stable reports whether v is a release tag without a prerelease part, e.g. v1.2.0.
func Stable(v string) bool { return stableRe.MatchString(v) }

// Newer reports whether latest is a newer stable version than cur; never for a non-release cur.
func Newer(cur, latest string) bool {
	c, err := Compare(cur, latest)
	return Stable(cur) && Stable(latest) && err == nil && c < 0
}

// Latest is the newest stable tag, read from the releases/latest redirect (no GitHub API limit).
func Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, LatestURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || loc.Path == "" {
		return "", fmt.Errorf("%s: no release redirect (HTTP %d)", LatestURL, resp.StatusCode)
	}
	tag := path.Base(loc.Path)
	if !Stable(tag) {
		return "", fmt.Errorf("%s: unexpected release tag %q", LatestURL, tag)
	}
	return tag, nil
}

// Package databases that mark /usr/bin/lodev as owned by a package.
var (
	DpkgInfoDir    = "/var/lib/dpkg/info"
	PacmanLocalDir = "/var/lib/pacman/local"
)

// Manager is the package manager that installed lodev; Name "" means a manual install.
type Manager struct{ Name, Command string }

// DetectManager says how the lodev at exe (symlinks already resolved) was installed.
func DetectManager(exe string) Manager {
	switch {
	case strings.Contains(exe, "/Cellar/"):
		return Manager{"Homebrew", "brew upgrade lodev"}
	case strings.HasPrefix(exe, "/nix/store/"):
		return Manager{"Nix", "update lodev with nix (e.g. nix profile upgrade lodev)"}
	}
	// The .deb and AUR packages install /usr/bin/lodev; a manual install elsewhere stays upgradable.
	if exe == "/usr/bin/lodev" {
		if _, err := os.Stat(filepath.Join(DpkgInfoDir, "lodev.list")); err == nil {
			return Manager{"apt", "download the .deb from https://github.com/namnh198/lodev/releases/latest and run: sudo apt install ./lodev_*.deb"}
		}
		if m, _ := filepath.Glob(filepath.Join(PacmanLocalDir, "lodev-bin-*")); len(m) > 0 {
			return Manager{"AUR", "update lodev-bin from the AUR (e.g. yay -Syu lodev-bin)"}
		}
	}
	return Manager{Command: "lodev self-upgrade"}
}

// CacheTTL is how long a check result is reused before GitHub is asked again.
const CacheTTL = 4 * time.Hour

// Cache is the last update check.
type Cache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

func (c Cache) Fresh(now time.Time) bool {
	return !c.CheckedAt.IsZero() && now.Sub(c.CheckedAt) < CacheTTL
}

// LoadCache reads the cache file; a missing or invalid one is the zero Cache.
func LoadCache(p string) Cache {
	var c Cache
	data, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(data, &c) != nil {
		return Cache{}
	}
	return c
}

// SaveCache writes the cache file atomically.
func SaveCache(p string, c Cache) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".update-check-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p)
}
