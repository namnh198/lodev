package dockertools

import (
	"fmt"
	"io"
	"runtime"
	"time"

	"charm.land/bubbles/v2/progress"
)

var now = time.Now

// wrapProgress prints the download line and, on a terminal with a known size, a progress bar
// redrawn in place as r is read.
func wrapProgress(t Tool, total int64, r io.Reader) io.Reader {
	fmt.Fprintf(progressOut, "Downloading %s %s (%s/%s)\n", t.Name(), t.Version(), runtime.GOOS, runtime.GOARCH)
	if total <= 0 || !isTerminal() {
		return r
	}
	return &progressReader{r: r, total: total, bar: progress.New(progress.WithDefaultBlend(), progress.WithWidth(30))}
}

type progressReader struct {
	r           io.Reader
	total, done int64
	bar         progress.Model
	drawn       time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if err == io.EOF {
		p.draw()
		fmt.Fprintln(progressOut)
	} else if now().Sub(p.drawn) >= 100*time.Millisecond {
		p.draw()
	}
	return n, err
}

func (p *progressReader) draw() {
	p.drawn = now()
	pct := min(float64(p.done)/float64(p.total), 1)
	fmt.Fprintf(progressOut, "\r%s  %.1f/%.1f MB", p.bar.ViewAs(pct), float64(p.done)/1e6, float64(p.total)/1e6)
}
