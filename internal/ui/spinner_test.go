package ui

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// setTTY makes stdout count as a terminal, as setup's buffers never are.
func setTTY(on bool) { out.decide(); out.tty = on }

func TestSpinPlain(t *testing.T) {
	o, _ := setup(t, false, false)
	l := Spin("waiting a healthy", "waiting b healthy")
	l.Fail(1, "b not healthy")
	l.Done(0, "a healthy")
	l.Stop()
	l.Stop()
	l.Done(0, "late")
	if got, want := o.String(), "waiting a healthy\nwaiting b healthy\n✗ b not healthy\n✓ a healthy\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSpinPlainWhenDebug(t *testing.T) {
	o, _ := setup(t, false, false)
	setTTY(true)
	SetDebug(true)
	l := Spin("waiting a healthy")
	l.Done(0, "a healthy")
	l.Stop()
	if got, want := o.String(), "waiting a healthy\n✓ a healthy\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func spinLive(t *testing.T, color bool) string {
	t.Helper()
	o, _ := setup(t, color, false)
	setTTY(true)
	old := spinEvery
	spinEvery = time.Millisecond
	t.Cleanup(func() { spinEvery = old })
	l := Spin("w a", "w b")
	time.Sleep(20 * time.Millisecond)
	l.Done(0, "a ok")
	l.Fail(1, "b bad")
	l.Stop()
	return o.String()
}

func TestSpinLive(t *testing.T) {
	got := spinLive(t, false)
	i := strings.LastIndex(got, "\x1b[2A")
	if i < 0 {
		t.Fatalf("no cursor-up redraw in %q", got)
	}
	if !strings.Contains(got, spinFrames[1]+" w a") {
		t.Errorf("no spinning frame in %q", got)
	}
	if last := got[i:]; !strings.HasSuffix(last, "✓ a ok\n\r\x1b[2K✗ b bad\n") {
		t.Errorf("final block %q", last)
	}
}

func TestSpinLiveColor(t *testing.T) {
	got := spinLive(t, true)
	for _, want := range []string{"\x1b[32m✓ a ok\x1b[0m\n", "\x1b[31m✗ b bad\x1b[0m\n", "\x1b[94m" + spinFrames[0] + "\x1b[0m w a"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestSpinConcurrent(t *testing.T) {
	setup(t, false, false)
	setTTY(true)
	old := spinEvery
	spinEvery = time.Millisecond
	t.Cleanup(func() { spinEvery = old })
	labels := make([]string, 20)
	for i := range labels {
		labels[i] = fmt.Sprint("w ", i)
	}
	l := Spin(labels...)
	var wg sync.WaitGroup
	for i := range labels {
		wg.Go(func() { time.Sleep(time.Millisecond); l.Done(i, "ok") })
	}
	wg.Wait()
	l.Stop()
}
