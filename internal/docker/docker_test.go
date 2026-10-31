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
	got := PHPBuildArgs([]string{"8.2", "8.3"}, 501, 20, "/tmp/x")
	want := []string{"build", "-t", "lodev/php", "--build-arg", "PHP_VERSIONS=8.2 8.3",
		"--build-arg", "UID=501", "--build-arg", "GID=20", "/tmp/x"}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestLegacyBuildArgs(t *testing.T) {
	check(t, LegacyBuildArgs("7.4", "/tmp/x"),
		"build -t lodev/php:legacy-7.4 --build-arg PHP_VERSION=7.4 -f /tmp/x/Dockerfile.legacy /tmp/x")
}
