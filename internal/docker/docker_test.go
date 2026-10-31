package docker

import (
	"slices"
	"strings"
	"testing"
)

func check(t *testing.T, got []string, want string) {
	t.Helper()
	if !slices.Equal(got, strings.Fields(want)) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestComposeArgs(t *testing.T) {
	check(t, ComposeArgs("lodev-shop", "/p/.lodev/compose.yaml", "up", "-d"),
		"compose -p lodev-shop -f /p/.lodev/compose.yaml up -d")
}

func TestExecArgsTTY(t *testing.T) {
	check(t, ExecArgs("P", "F", "www-data", true, []string{"php", "-v"}),
		"compose -p P -f F exec -u www-data -w /var/www/html web php -v")
}

func TestExecArgsNoTTY(t *testing.T) {
	check(t, ExecArgs("P", "F", "www-data", false, []string{"php", "-v"}),
		"compose -p P -f F exec -T -u www-data -w /var/www/html web php -v")
}

func TestPHPBuildArgs(t *testing.T) {
	got := PHPBuildArgs([]string{"8.2", "8.3"}, 501, 20, "H", false, "/tmp/x")
	want := []string{"build", "-t", "lodev/php", "--label", "lodev.hash=H", "--build-arg", "PHP_VERSIONS=8.2 8.3",
		"--build-arg", "UID=501", "--build-arg", "GID=20", "/tmp/x"}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got := PHPBuildArgs([]string{"8.2"}, 501, 20, "H", true, "/tmp/x"); got[1] != "--no-cache" {
		t.Fatalf("rebuild must not use the cache: %q", got)
	}
}

func TestLegacyBuildArgs(t *testing.T) {
	check(t, LegacyBuildArgs("7.4", "sha256:B", false, "/tmp/x"),
		"build -t lodev/php:legacy-7.4 --label lodev.base=sha256:B --build-arg PHP_VERSION=7.4 -f /tmp/x/Dockerfile.legacy /tmp/x")
	if got := LegacyBuildArgs("7.4", "B", true, "/tmp/x"); got[1] != "--no-cache" {
		t.Fatalf("rebuild must not use the cache: %q", got)
	}
}

// fakeImages replaces docker inspect/build with an in-memory image store.
func fakeImages(t *testing.T, labels map[string]string) *[]string {
	t.Helper()
	var built []string
	oi, ob, oh := inspect, build, phpHash
	t.Cleanup(func() { inspect, build, phpHash = oi, ob, oh })
	phpHash = func() (string, error) { return "H", nil }
	inspect = func(format, tag string) string {
		if format == "{{.Id}}" {
			return labels[tag+"#id"]
		}
		return labels[tag]
	}
	build = func(sub string, args func(string) []string) error {
		a := args("/ctx")
		built = append(built, a[slices.Index(a, "-t")+1])
		return nil
	}
	return &built
}

func TestEnsurePHPImage(t *testing.T) {
	cases := []struct {
		name    string
		ver     string
		rebuild bool
		labels  map[string]string // tag → its lodev label; tag#id → image id
		want    []string
	}{
		{"base current", "8.3", false, map[string]string{"lodev/php": "H"}, nil},
		{"base missing", "8.3", false, map[string]string{}, []string{"lodev/php"}},
		{"base stale", "8.3", false, map[string]string{"lodev/php": "OLD"}, []string{"lodev/php"}},
		{"legacy current", "7.4", false, map[string]string{"lodev/php": "H", "lodev/php#id": "B", "lodev/php:legacy-7.4": "B"}, nil},
		{"legacy missing", "7.4", false, map[string]string{"lodev/php": "H", "lodev/php#id": "B"}, []string{"lodev/php:legacy-7.4"}},
		{"legacy on old base", "7.4", false, map[string]string{"lodev/php": "H", "lodev/php#id": "B", "lodev/php:legacy-7.4": "A"}, []string{"lodev/php:legacy-7.4"}},
		{"rebuild", "7.4", true, map[string]string{"lodev/php": "H", "lodev/php#id": "B", "lodev/php:legacy-7.4": "B"}, []string{"lodev/php", "lodev/php:legacy-7.4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			built := fakeImages(t, c.labels)
			if err := EnsurePHPImage(c.ver, c.rebuild); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(*built, c.want) {
				t.Fatalf("built %v, want %v", *built, c.want)
			}
		})
	}
}
