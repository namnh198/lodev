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
	"strconv"
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
	if !Stable(cur) || !Stable(latest) {
		return false
	}
	a, b := strings.Split(cur[1:], "."), strings.Split(latest[1:], ".")
	for i := range a {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return y > x
		}
	}
	return false
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
