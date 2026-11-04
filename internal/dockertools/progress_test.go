package dockertools

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
)

func readThrough(t *testing.T, terminal bool, total int64, size int) string {
	t.Helper()
	var out bytes.Buffer
	oldOut, oldTerm := progressOut, isTerminal
	progressOut, isTerminal = &out, func() bool { return terminal }
	defer func() { progressOut, isTerminal = oldOut, oldTerm }()
	if _, err := io.Copy(io.Discard, wrapProgress(Compose, total, bytes.NewReader(make([]byte, size)))); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

var downloadLine = fmt.Sprintf("Downloading docker-compose v5.7.0 (%s/%s)\n", runtime.GOOS, runtime.GOARCH)

func TestProgressBar(t *testing.T) {
	got := readThrough(t, true, 3_000_000, 3_000_000)
	for _, want := range []string{downloadLine, "100%", "3.0/3.0 MB"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q: %q", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") || !strings.Contains(got, "\r") {
		t.Errorf("bar must redraw with \\r and end with a newline: %q", got)
	}
}

func TestProgressNoTerminal(t *testing.T) {
	if got := readThrough(t, false, 3_000_000, 3_000_000); got != downloadLine {
		t.Errorf("got %q", got)
	}
}

func TestProgressUnknownSize(t *testing.T) {
	if got := readThrough(t, true, -1, 1000); got != downloadLine {
		t.Errorf("got %q", got)
	}
}
