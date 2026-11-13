package cmd

import (
	"errors"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// TestWarnWorkDirMove: a web container made for another working_dir gets one warning before it is recreated.
func TestWarnWorkDirMove(t *testing.T) {
	old := dockerOutput
	t.Cleanup(func() { dockerOutput = old })
	for _, c := range []struct {
		current string
		err     error
		warn    bool
	}{
		{"/var/www/html", nil, true},
		{"/app", nil, false},
		{"", errors.New("No such object"), false},
	} {
		_, e, restore := ui.Capture(false)
		dockerOutput = func(a ...string) (string, error) {
			if strings.Join(a, " ") != "inspect --format {{.Config.WorkingDir}} lodev-shop-web" {
				t.Fatalf("docker %q", a)
			}
			return c.current, c.err
		}
		warnWorkDirMove(&config.Project{Name: "shop"})
		restore()
		got := e.String()
		if c.warn != (got != "") {
			t.Errorf("%q: warning %q", c.current, got)
		}
		if c.warn && !strings.Contains(got, "/var/www/html → /app") {
			t.Errorf("warning lacks the move: %q", got)
		}
	}
}
