package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeUsesEnv(t *testing.T) {
	t.Setenv("LODEV_HOME", "/x")
	if Home() != "/x" {
		t.Fatal(Home())
	}
}

func TestHomeDefault(t *testing.T) {
	t.Setenv("LODEV_HOME", "")
	h, _ := os.UserHomeDir()
	if Home() != filepath.Join(h, ".lodev") {
		t.Fatal(Home())
	}
}
