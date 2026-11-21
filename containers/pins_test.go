package containers

import (
	"regexp"
	"strings"
	"testing"

	"lodev/internal/config"
)

// Every download in lodev's Dockerfiles is pinned and checked: a moved "latest" or a swapped file
// would otherwise land in the image (or in the CA mkcert makes) unnoticed, and stay cached forever.
func TestDownloadsPinned(t *testing.T) {
	curl := regexp.MustCompile(`curl -fsSL -o (\S+) (\S+)`)
	from := regexp.MustCompile(`COPY --from=(\S+)`)
	for _, f := range []string{"webserver/Dockerfile", "mkcert/Dockerfile"} {
		b, err := FS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, m := range curl.FindAllStringSubmatch(s, -1) {
			path, url := m[1], m[2]
			if !strings.Contains(s, path+`" | sha256sum -c`) {
				t.Errorf("%s: %s is downloaded without a sha256sum -c check", f, path)
			}
			if strings.Contains(url, "latest") || strings.Contains(url, "gh-pages") {
				t.Errorf("%s: %s is not a pinned version", f, url)
			}
		}
		for _, m := range from.FindAllStringSubmatch(s, -1) {
			if _, tag, ok := strings.Cut(m[1], ":"); !ok || !strings.Contains(tag, ".") {
				t.Errorf("%s: COPY --from=%s has no pinned version tag", f, m[1])
			}
		}
	}
}

// The web image carries build tools, service clients and pinned Node CLIs.
func TestWebImageTools(t *testing.T) {
	b, err := FS.ReadFile("webserver/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, w := range []string{"redis-tools", "gcc", "g++", "make", "patch"} {
		if !strings.Contains(s, w) {
			t.Errorf("webserver/Dockerfile does not mention %s", w)
		}
	}
	install := regexp.MustCompile(`npm install -g ([^\n\\&]+)`).FindStringSubmatch(s)
	if install == nil {
		t.Fatal("no npm install -g in webserver/Dockerfile")
	}
	for _, pkg := range []string{"pnpm", "yarn", "grunt-cli"} {
		if !regexp.MustCompile(`(^|\s)` + pkg + `@\d+\.\d+\.\d+(\s|$)`).MatchString(install[1]) {
			t.Errorf("npm install -g does not pin %s to an exact version: %q", pkg, install[1])
		}
	}
	php, err := FS.ReadFile("webserver/install-php.sh")
	if err != nil {
		t.Fatal(err)
	}
	// php$v-dev: phpize and pecl install build against the active version's headers.
	if !regexp.MustCompile(`for ext in [^;]*\bdev\b`).Match(php) {
		t.Error("install-php.sh does not install php$v-dev")
	}
	// php-pear (pecl) is installed with the PHP packages, after the sury source exists; the
	// Dockerfile's first apt-get runs before it.
	if !regexp.MustCompile(`pkgs=\([^)]*\bphp-pear\b`).Match(php) {
		t.Error("install-php.sh does not install php-pear")
	}
	if strings.Contains(s, "php-pear") {
		t.Error("webserver/Dockerfile installs php-pear before the sury source exists")
	}
}

// Every supported PHP has a minimum package version in install-php.sh (the PHP2Shell fixes,
// CVE-2026-109618/109619), so a build never ships an unpatched PHP from a stale mirror or cache.
func TestPHPPatchedFloor(t *testing.T) {
	b, err := FS.ReadFile("webserver/install-php.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range config.SupportedPHP {
		if !regexp.MustCompile(`\n\s*` + regexp.QuoteMeta(v) + `\) min=\S+ ;;`).Match(b) {
			t.Errorf("install-php.sh has no minimum version for PHP %s", v)
		}
	}
	if !strings.Contains(string(b), `dpkg --compare-versions "$have" ge "$min"`) {
		t.Error("install-php.sh does not check the installed version against its minimum")
	}
}
