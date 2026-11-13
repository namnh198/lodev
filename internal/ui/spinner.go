package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
)

// Swapped in tests.
var (
	spinFrames = spinner.MiniDot.Frames
	spinEvery  = spinner.MiniDot.FPS
)

const (
	waiting = iota
	done
	failed
)

// Lines shows one status line per label on stdout: spinning while waiting, then ✓ or ✗.
// On a terminal (debug off) the block is redrawn in place; otherwise each change is a new line.
type Lines struct {
	mu            sync.Mutex
	labels, msgs  []string
	state         []int
	frame         int
	live, stopped bool
	stop, exited  chan struct{}
}

// Spin prints the waiting lines and, on a terminal, starts animating them.
func Spin(labels ...string) *Lines {
	l := &Lines{labels: labels, msgs: make([]string, len(labels)), state: make([]int, len(labels)),
		live: len(labels) > 0 && out.isTerm() && !debug} // debug lines on stderr would break the redraw
	if !l.live {
		for _, s := range labels {
			fmt.Fprintln(out.w, s)
		}
		return l
	}
	l.stop, l.exited = make(chan struct{}), make(chan struct{})
	l.draw(false)
	go l.tick()
	return l
}

func (l *Lines) tick() {
	defer close(l.exited)
	t := time.NewTicker(spinEvery)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			l.mu.Lock()
			l.frame++
			l.draw(true)
			l.mu.Unlock()
		}
	}
}

// draw writes the block; again first moves the cursor up over the previous one.
func (l *Lines) draw(again bool) {
	var b strings.Builder
	if again {
		fmt.Fprintf(&b, "\x1b[%dA", len(l.labels))
	}
	for i := range l.labels {
		fmt.Fprintf(&b, "\r\x1b[2K%s\n", l.line(i))
	}
	fmt.Fprint(out.w, b.String())
}

func (l *Lines) line(i int) string {
	switch l.state[i] {
	case done:
		return paint("32", "✓ "+l.msgs[i])
	case failed:
		return paint("31", "✗ "+l.msgs[i])
	}
	return paint("94", spinFrames[l.frame%len(spinFrames)]) + " " + l.labels[i]
}

func paint(code, s string) string {
	if out.useColor() {
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
	return s
}

// Done turns line i into "✓ msg"; Fail into "✗ msg". Calls after Stop, or on a finished line, are ignored.
func (l *Lines) Done(i int, msg string) { l.finish(i, done, msg) }
func (l *Lines) Fail(i int, msg string) { l.finish(i, failed, msg) }

func (l *Lines) finish(i, st int, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped || l.state[i] != waiting {
		return
	}
	l.state[i], l.msgs[i] = st, msg
	if !l.live {
		fmt.Fprintln(out.w, l.line(i))
	}
}

// Stop ends the animation with a last redraw; calling it again does nothing.
func (l *Lines) Stop() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.stopped = true
	l.mu.Unlock()
	if !l.live {
		return
	}
	close(l.stop)
	<-l.exited // the ticker takes the lock, so wait for it unlocked
	l.mu.Lock()
	l.draw(true)
	l.mu.Unlock()
}
