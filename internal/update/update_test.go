package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	for _, tt := range []struct {
		cur, latest string
		want        bool
	}{
		{"v1.2.0", "v1.3.0", true}, {"v1.9.0", "v1.10.0", true}, {"v1.3.0", "v1.3.0", false},
		{"v1.4.0", "v1.3.0", false}, {"v2.0.0", "v1.9.9", false}, {"dev", "v1.3.0", false},
		{"dev (8a7cb83)", "v1.3.0", false}, {"v1.3.0-rc1", "v1.3.0", false}, {"v1.2.0", "", false},
	} {
		if got := Newer(tt.cur, tt.latest); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v", tt.cur, tt.latest, got)
		}
	}
}

// serveLatest points LatestURL at a server whose HEAD answers with h.
func serveLatest(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	old := LatestURL
	LatestURL = srv.URL + "/releases/latest"
	t.Cleanup(func() { LatestURL = old })
}

func TestLatest(t *testing.T) {
	for loc, want := range map[string]string{
		"https://github.com/namnh198/lodev/releases/tag/v1.3.0":  "v1.3.0",
		"https://github.com/namnh198/lodev/releases/tag/nightly": "",
		"": "",
	} {
		serveLatest(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodHead {
				t.Errorf("method %s", r.Method)
			}
			if loc != "" {
				w.Header().Set("Location", loc)
				w.WriteHeader(http.StatusFound)
			}
		})
		got, err := Latest(context.Background())
		if want == "" {
			if err == nil {
				t.Errorf("Location %q: want error, got %q", loc, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("Location %q: got %q, %v", loc, got, err)
		}
	}
}

func TestLatestTimeout(t *testing.T) {
	serveLatest(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Latest(ctx); err == nil {
		t.Error("want timeout error")
	}
}

func TestCacheFresh(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	if !(Cache{CheckedAt: now.Add(-3*time.Hour - 59*time.Minute)}).Fresh(now) {
		t.Error("3h59m old should be fresh")
	}
	if (Cache{CheckedAt: now.Add(-4 * time.Hour)}).Fresh(now) {
		t.Error("4h old should be stale")
	}
	if (Cache{}).Fresh(now) {
		t.Error("zero cache should be stale")
	}
}

func TestCacheRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "update-check.json")
	c := Cache{CheckedAt: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC), Latest: "v1.3.0"}
	if err := SaveCache(p, c); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(p); !got.CheckedAt.Equal(c.CheckedAt) || got.Latest != c.Latest {
		t.Errorf("round trip: %+v", got)
	}
	if err := os.WriteFile(p, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(p); got != (Cache{}) {
		t.Errorf("invalid file: %+v", got)
	}
	if got := LoadCache(p + ".missing"); got != (Cache{}) {
		t.Errorf("missing file: %+v", got)
	}
}
