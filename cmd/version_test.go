package cmd

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestLodevVersion(t *testing.T) {
	defer func(o string) { Version = o }(Version)
	Version = "v1.2.0"
	if got := lodevVersion(); got != "v1.2.0" {
		t.Errorf("ldflags version: %q", got)
	}
	Version = "dev"
	if got := lodevVersion(); !strings.HasPrefix(got, "dev") {
		t.Errorf("dev version: %q", got)
	}
}

func TestVersionRows(t *testing.T) {
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)
	dockerOutput = func(a ...string) (string, error) {
		return map[string]string{
			"version": "29.8.2",
			"compose": "5.5.0",
			"buildx":  "github.com/docker/buildx v0.37.2 2d37",
		}[a[0]], nil
	}
	rows := versionRows()
	var names, values []string
	for _, r := range rows {
		names, values = append(names, r[0]), append(values, r[1])
	}
	if want := []string{"lodev", "Docker", "Docker Compose", "Docker Buildx", "OS", "Architecture"}; !slices.Equal(names, want) {
		t.Fatalf("names %v", names)
	}
	if want := []string{"29.8.2", "5.5.0", "v0.37.2", runtime.GOOS, runtime.GOARCH}; !slices.Equal(values[1:], want) {
		t.Errorf("values %v", values)
	}

	dockerOutput = func(...string) (string, error) { return "", errors.New("Cannot connect to the Docker daemon") }
	rows = versionRows()
	for _, r := range rows[1:4] {
		if r[1] != "unavailable" {
			t.Errorf("%s = %q with Docker down", r[0], r[1])
		}
	}
}
