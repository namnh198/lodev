package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/registry"
)

func TestShortPath(t *testing.T) {
	t.Setenv("HOME", "/Users/u")
	for in, want := range map[string]string{
		"/Users/u/a/b": "~/a/b",
		"/Users/u":     "~",
		"/Users/uX/p":  "/Users/uX/p",
		"/opt/x":       "/opt/x",
	} {
		if got := shortPath(in); got != want {
			t.Errorf("shortPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderTable(t *testing.T) {
	out := renderTable([]string{"A", "B"}, [][]string{{"x\ny", "z"}})
	if !strings.Contains(out, "╭") || !strings.Contains(out, "A") || !strings.Contains(out, "z") {
		t.Fatalf("no border/header/cell:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	xi, yi := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "x") && xi < 0 {
			xi = i
		}
		if strings.Contains(l, "y") {
			yi = i
		}
	}
	if xi < 0 || yi <= xi {
		t.Fatalf("multi-line cell not split:\n%s", out)
	}
}

func TestListTable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LODEV_HOME", t.TempDir())
	dir := filepath.Join(home, "sites", "shop")
	p, err := newProjectConfig(dir, "shop", "php", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	registry.Registry{"shop": {Path: dir, Domains: []string{"shop.test", "en.test"}}}.Save()
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)
	dockerOutput = func(...string) (string, error) { return "running", nil }

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"list"})
	defer func() { rootCmd.SetOut(nil); rootCmd.SetArgs(nil) }()
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "~/sites/shop") || strings.Contains(out, home) {
		t.Errorf("path not shortened:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("ANSI codes in non-terminal output:\n%q", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "shop.test") && strings.Contains(l, "en.test") {
			t.Errorf("domains share a line:\n%s", out)
		}
	}
	if !strings.Contains(out, "shop.test") || !strings.Contains(out, "en.test") {
		t.Errorf("domains missing:\n%s", out)
	}
}
