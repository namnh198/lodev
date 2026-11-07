package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"lodev/assets"
	"lodev/internal/config"
	"lodev/internal/dockertools"
)

// TestMain keeps every other test away from the first-run setup and the real Docker daemon.
func TestMain(m *testing.M) {
	firstRun = func() {}
	waitHealthyFn = func(string, time.Duration, string) error { return nil }
	waitServicesFn = func([]string, time.Duration) error { return nil }
	dockerPing = func() error { return nil }
	os.Exit(m.Run())
}

func TestGlobalConfigTemplateIsTheDefaults(t *testing.T) {
	data, err := assets.FS.ReadFile("templates/global-config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.ParseGlobal(data)
	want, _ := config.ParseGlobal(nil)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("template drifted from the defaults (%v):\n%+v\n%+v", err, got, want)
	}
}

// stubSetup records what the first-run setup does instead of downloading or running Docker.
func stubSetup(t *testing.T, mkcert bool, ping error) *[]string {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	od, oc, op, om := setupDownload, setupCA, dockerPing, hasMkcert
	t.Cleanup(func() { setupDownload, setupCA, dockerPing, hasMkcert = od, oc, op, om })
	var did []string
	setupDownload = func(tl dockertools.Tool) (string, error) { did = append(did, "download "+tl.Name()); return "", nil }
	setupCA = func() error { did = append(did, "ca"); return nil }
	dockerPing = func() error { return ping }
	hasMkcert = func() bool { return mkcert }
	return &did
}

func TestFirstRunSetup(t *testing.T) {
	did := stubSetup(t, false, nil)
	firstRunSetup()
	for _, f := range []string{config.GlobalPath(), filepath.Join(config.Home(), "projects.json")} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s not created: %v", f, err)
		}
	}
	if fi, err := os.Stat(filepath.Join(config.Home(), "data")); err != nil || !fi.IsDir() {
		t.Errorf("data dir: %v", err)
	}
	if want := []string{"download docker-compose", "download docker-buildx", "ca"}; !slices.Equal(*did, want) {
		t.Errorf("did %q, want %q", *did, want)
	}
	*did = nil
	firstRunSetup()
	if len(*did) != 0 {
		t.Errorf("second run did %q", *did)
	}
}

func TestFirstRunSetupWithMkcert(t *testing.T) {
	did := stubSetup(t, true, nil)
	firstRunSetup()
	if slices.Contains(*did, "ca") {
		t.Errorf("CA made although mkcert is installed: %q", *did)
	}
}

func TestFirstRunSetupDockerDownAndFailures(t *testing.T) {
	did := stubSetup(t, false, errors.New("down"))
	setupDownload = func(dockertools.Tool) (string, error) { return "", errors.New("offline") }
	firstRunSetup()
	if slices.Contains(*did, "ca") {
		t.Errorf("CA attempted without Docker: %q", *did)
	}
	if _, err := os.Stat(config.GlobalPath()); err != nil {
		t.Errorf("a failed download must not stop the setup: %v", err)
	}
}

func TestSkipsSetup(t *testing.T) {
	for _, path := range [][]string{{"help"}, {"version"}, {"completion", "bash"}, {"self-upgrade"}} {
		c, _, err := rootCmd.Find(path)
		if err != nil || !skipsSetup(c) {
			t.Errorf("%v: setup not skipped (%v)", path, err)
		}
	}
	if c, _, _ := rootCmd.Find([]string{"list"}); skipsSetup(c) {
		t.Error("list skips the setup")
	}
}

// Every command must be classified, so a new one cannot silently skip the Docker check.
func TestNeedsDockerCoversEveryCommand(t *testing.T) {
	noDocker := map[string]bool{
		"lodev create": true, "lodev list": true, "lodev describe": true, "lodev config": true, "lodev config-global": true,
		"lodev dotenv set": true, "lodev dotenv get": true, "lodev version": true, "lodev self-upgrade": true,
	}
	var unknown []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range c.Commands() {
			walk(s)
		}
		p := c.CommandPath()
		if !c.Runnable() || strings.HasPrefix(p, "lodev completion") || strings.HasPrefix(p, "lodev help") || strings.HasPrefix(p, "lodev __") {
			return
		}
		if needsDocker[p] == noDocker[p] {
			unknown = append(unknown, p)
		}
	}
	walk(rootCmd)
	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Errorf("classify these in needsDocker (or the test's noDocker): %q", unknown)
	}
}

func TestDockerDownStopsStart(t *testing.T) {
	root := newShop(t)
	dockerPing = func() error { return errors.New("Docker is not running") }
	t.Cleanup(func() { dockerPing = func() error { return nil } })
	rootCmd.SetArgs([]string{"start"})
	if err := rootCmd.Execute(); err == nil || err.Error() != "Docker is not running" {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".lodev", "lodev.compose-base.yml")); err == nil {
		t.Error("start wrote files with Docker down")
	}
}
