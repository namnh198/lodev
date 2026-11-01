package ui

import (
	"bytes"
	"testing"
)

func setup(t *testing.T, outColor, errColor bool) (out, err *bytes.Buffer) {
	t.Helper()
	out, err = &bytes.Buffer{}, &bytes.Buffer{}
	setOutput(out, err, outColor, errColor)
	SetDebug(false)
	t.Cleanup(func() { setOutput(nil, nil, false, false); SetDebug(false) })
	return
}

func TestLevels(t *testing.T) {
	out, err := setup(t, true, true)
	Error("x")
	Warn("x")
	if got := err.String(); got != "\x1b[31merror: x\x1b[0m\n\x1b[33mwarning: x\x1b[0m\n" {
		t.Fatalf("stderr %q", got)
	}
	err.Reset()
	Success("x")
	Info("x")
	if got := out.String(); got != "\x1b[32mx\x1b[0m\n\x1b[94mx\x1b[0m\n" {
		t.Fatalf("stdout %q", got)
	}
	Debug("x")
	if err.Len() != 0 {
		t.Fatalf("debug printed while off: %q", err.String())
	}
	SetDebug(true)
	Debug("x")
	if got := err.String(); got != "\x1b[90mx\x1b[0m\n" {
		t.Fatalf("debug %q", got)
	}
}

func TestNoColor(t *testing.T) {
	out, err := setup(t, false, false)
	SetDebug(true)
	Error("x")
	Warn("x")
	Debug("x")
	Success("x")
	Info("x")
	if err.String() != "error: x\nwarning: x\nx\n" || out.String() != "x\nx\n" {
		t.Fatalf("out %q err %q", out.String(), err.String())
	}
}

func TestMultiLine(t *testing.T) {
	out, _ := setup(t, true, true)
	Info("a\nb\n")
	if got := out.String(); got != "\x1b[94ma\x1b[0m\n\x1b[94mb\x1b[0m\n" {
		t.Fatalf("got %q", got)
	}
}

func TestPerStreamColor(t *testing.T) {
	out, err := setup(t, false, true)
	Info("x")
	Warn("x")
	if out.String() != "x\n" || err.String() != "\x1b[33mwarning: x\x1b[0m\n" {
		t.Fatalf("out %q err %q", out.String(), err.String())
	}
}

func TestColorFor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colorFor(true) {
		t.Fatal("NO_COLOR must disable color")
	}
	t.Setenv("NO_COLOR", "")
	if !colorFor(true) || colorFor(false) {
		t.Fatal("color should follow terminal-ness")
	}
}

func TestCapture(t *testing.T) {
	out, err, restore := Capture(true)
	Info("i")
	Debug("d")
	restore()
	if out.String() != "i\n" || err.String() != "d\n" {
		t.Fatalf("out %q err %q", out.String(), err.String())
	}
}
