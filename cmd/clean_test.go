package cmd

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/registry"
)

func TestCleanKeep(t *testing.T) {
	g, _ := config.ParseGlobal(nil)
	keep := cleanKeep(g, registry.Registry{"shop": {}})
	for _, v := range config.SupportedPHP {
		if !keep[config.PHPImage(v)] {
			t.Errorf("%s not kept", config.PHPImage(v))
		}
	}
	for _, ref := range []string{"lodev/opensearch:2", "lodev/mkcert:latest", "lodev/shop-web:latest"} {
		if !keep[ref] {
			t.Errorf("%s not kept", ref)
		}
	}
}

func TestCleanCandidates(t *testing.T) {
	cur := config.PHPImage("8.4")
	imgs := []imageInfo{
		{"lodev/webserver:v0.1.0", "aaa111", "1.1GB"}, {"lodev/webserver:v0.1.0-legacy-7.4", "bbb222", "300MB"},
		{"lodev/php:latest", "ccc333", "1GB"}, {"lodev/gone-web:latest", "ddd444", "1.2GB"},
		{"nginx:latest", "eee555", "190MB"}, {cur, "fff666", "1.1GB"}, {"lodev/shop-web:latest", "ggg777", "1.3GB"},
	}
	keep := map[string]bool{cur: true, "lodev/shop-web:latest": true}
	var got []string
	for _, c := range cleanCandidates(imgs, nil, keep) {
		got = append(got, c.Ref)
	}
	want := []string{"lodev/webserver:v0.1.0", "lodev/webserver:v0.1.0-legacy-7.4", "lodev/php:latest", "lodev/gone-web:latest"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestCleanKeepsUsedImages(t *testing.T) {
	imgs := []imageInfo{{"lodev/old-web:latest", "aaa111", "1GB"}, {"lodev/webserver:v0.0.9", "bbb222bbb222", "1GB"}, {"lodev/php:latest", "ccc333", "1GB"}}
	// ps -a lists a stopped container's image by ref, and by short ID once its tag moved.
	used := map[string]bool{"lodev/old-web": true, "bbb222bbb222": true}
	got := cleanCandidates(imgs, used, nil)
	if len(got) != 1 || got[0].Ref != "lodev/php:latest" {
		t.Errorf("got %+v", got)
	}
}

func stubClean(t *testing.T, rmiFail string) *[]string {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	oldOut, oldRun := dockerOutput, dockerRun
	t.Cleanup(func() { dockerOutput, dockerRun = oldOut, oldRun })
	var calls []string
	dockerOutput = func(a ...string) (string, error) {
		switch a[0] {
		case "images":
			return "lodev/php:latest\tccc333\t1GB\nlodev/webserver:v0.0.1\tddd444\t1.1GB", nil
		case "ps":
			return "", nil
		case "system":
			return "Images\t3GB\nBuild Cache\t2.5GB", nil
		case "rmi":
			calls = append(calls, strings.Join(a, " "))
			if a[1] == rmiFail {
				return "", errors.New("image is being used")
			}
		}
		return "", nil
	}
	dockerRun = func(a ...string) error { calls = append(calls, strings.Join(a, " ")); return nil }
	return &calls
}

func TestCleanCommand(t *testing.T) {
	calls := stubClean(t, "")
	out, _, err := runConfig(t, newCleanCmd, "--dry-run")
	if err != nil || len(*calls) != 0 || !strings.Contains(out, "lodev/php:latest") || !strings.Contains(out, "2.5GB") ||
		!strings.Contains(out, "shared by every project on this machine") {
		t.Errorf("--dry-run: %q %q %v", out, *calls, err)
	}
	calls = stubClean(t, "")
	if _, _, err := runConfig(t, newCleanCmd, "--yes"); err != nil ||
		!slices.Equal(*calls, []string{"rmi lodev/php:latest", "rmi lodev/webserver:v0.0.1", "builder prune -f --filter until=168h"}) {
		t.Errorf("--yes: %q %v", *calls, err)
	}
	calls = stubClean(t, "")
	if _, _, err := runConfig(t, newCleanCmd, "--yes", "--keep-cache"); err != nil || slices.Contains(*calls, "builder prune -f --filter until=168h") {
		t.Errorf("--keep-cache: %q %v", *calls, err)
	}
	calls = stubClean(t, "lodev/php:latest")
	out, _, err = runConfig(t, newCleanCmd, "--yes")
	if err != nil || !slices.Contains(*calls, "rmi lodev/webserver:v0.0.1") {
		t.Errorf("failing rmi stopped the rest: %q %v", *calls, err)
	}
}
