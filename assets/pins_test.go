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
