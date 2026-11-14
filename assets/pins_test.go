package assets

import (
	"regexp"
	"strings"
	"testing"
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
	for _, w := range []string{"php-pear", "redis-tools", "gcc", "g++", "make", "mongosh"} {
		if !strings.Contains(s, w) {
			t.Errorf("webserver/Dockerfile does not mention %s", w)
		}
	}
	if !strings.Contains(s, `mongosh.tgz" | sha256sum -c`) {
		t.Error("mongosh is installed without a sha256sum -c check")
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
}
