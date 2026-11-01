// Package ui prints leveled, colored messages. It must not import other lodev packages.
package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// stream decides lazily, on first write, whether its writer gets color.
type stream struct {
	w       io.Writer
	color   bool
	decided bool
}

func (s *stream) useColor() bool {
	if !s.decided {
		s.decided = true
		if f, ok := s.w.(*os.File); ok {
			s.color = colorFor(term.IsTerminal(int(f.Fd())))
		}
	}
	return s.color
}

var (
	out   = &stream{w: os.Stdout}
	err   = &stream{w: os.Stderr}
	debug = os.Getenv("LODEV_DEBUG") == "1"
)

func colorFor(isTerm bool) bool { return isTerm && os.Getenv("NO_COLOR") == "" }

// setOutput replaces the writers and fixes their color; nil writers restore the defaults.
func setOutput(o, e io.Writer, outColor, errColor bool) {
	if o == nil && e == nil {
		out, err = &stream{w: os.Stdout}, &stream{w: os.Stderr}
		return
	}
	out = &stream{w: o, color: outColor, decided: true}
	err = &stream{w: e, color: errColor, decided: true}
}

// Capture redirects output (color off) into buffers for tests.
func Capture(dbg bool) (o, e *bytes.Buffer, restore func()) {
	oldOut, oldErr, oldDebug := out, err, debug
	o, e = &bytes.Buffer{}, &bytes.Buffer{}
	setOutput(o, e, false, false)
	debug = dbg
	return o, e, func() { out, err, debug = oldOut, oldErr, oldDebug }
}

func SetDebug(on bool) { debug = on }

func print(s *stream, code, prefix, format string, a ...any) {
	msg := strings.TrimSuffix(fmt.Sprintf(format, a...), "\n")
	for _, line := range strings.Split(msg, "\n") {
		line = prefix + line
		if s.useColor() {
			line = "\x1b[" + code + "m" + line + "\x1b[0m"
		}
		fmt.Fprintln(s.w, line)
	}
}

func Error(format string, a ...any)   { print(err, "31", "error: ", format, a...) }
func Warn(format string, a ...any)    { print(err, "33", "warning: ", format, a...) }
func Success(format string, a ...any) { print(out, "32", "", format, a...) }
func Info(format string, a ...any)    { print(out, "94", "", format, a...) }

func Debug(format string, a ...any) {
	if debug {
		print(err, "90", "", format, a...)
	}
}
