package cmd

import (
	"errors"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/dockertools"
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
	t.Setenv("LODEV_HOME", t.TempDir())
	writeGlobalFile(t, "use_docker_compose_system: true\nuse_docker_buildx_system: true\n")
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
	if want := []string{"29.8.2", "v5.5.0 (system)", "v0.37.2 (system)", runtime.GOOS, runtime.GOARCH}; !slices.Equal(values[1:], want) {
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

func TestVersionFlag(t *testing.T) {
	var out strings.Builder
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"-v"})
	defer func() { rootCmd.SetOut(nil); rootCmd.SetArgs(nil) }()
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := "lodev version " + lodevVersion() + "\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func writeGlobalFile(t *testing.T, body string) {
	t.Helper()
	os.MkdirAll(config.Home(), 0o755)
	if err := os.WriteFile(config.GlobalPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lodev version must never start a download of the managed tools.
func TestVersionRowsManaged(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir()) // nothing downloaded here
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)
	var calls []string
	dockerOutput = func(a ...string) (string, error) { calls = append(calls, a[0]); return "29.8.2", nil }
	got := map[string]string{}
	for _, r := range versionRows() {
		got[r[0]] = r[1]
	}
	if got["Docker Compose"] != "v5.7.0 (lodev)" || got["Docker Buildx"] != "v0.38.2 (lodev)" {
		t.Errorf("rows %v", got)
	}
	for _, c := range calls {
		if c == "compose" || c == "buildx" {
			t.Errorf("docker %s called", c)
		}
	}
	if _, err := os.Stat(dockertools.File(dockertools.Compose)); !os.IsNotExist(err) {
		t.Errorf("version downloaded docker-compose: %v", err)
	}
}
