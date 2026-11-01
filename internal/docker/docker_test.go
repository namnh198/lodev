package docker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/templates"
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

func TestExecArgs(t *testing.T) {
	check(t, ExecArgs("P", "F", "web", "kuroko", "/var/www/html", true, []string{"php", "-v"}),
		"compose -p P -f F exec -u kuroko -w /var/www/html web php -v")
	check(t, ExecArgs("P", "F", "adminer", "", "", false, []string{"ls"}),
		"compose -p P -f F exec -T adminer ls")
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
	oi, ob, oh, or := inspect, build, phpHash, run
	t.Cleanup(func() { inspect, build, phpHash, run = oi, ob, oh, or })
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

func TestProjectBuildArgs(t *testing.T) {
	check(t, ProjectBuildArgs("lodev/shop-web", "H", "/p/Dockerfile", "/p/ctx", false),
		"build -t lodev/shop-web --label lodev.hash=H -f /p/Dockerfile /p/ctx")
	if got := ProjectBuildArgs("t", "H", "f", "c", true); got[1] != "--no-cache" {
		t.Fatalf("rebuild must not use the cache: %q", got)
	}
}

func writeFiles(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWebImageParts(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, ".lodev/web-images/Dockerfile", ".lodev/web-images/Dockerfile.b",
		".lodev/web-images/Dockerfile.example", ".lodev/web-images/notes.txt", ".lodev/web-images/sub/Dockerfile")
	parts, err := WebImageParts(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range parts {
		names = append(names, p.Name)
	}
	want := []string{".lodev/web-images/Dockerfile", ".lodev/web-images/Dockerfile.b"}
	if !slices.Equal(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	if string(parts[0].Content) != "x .lodev/web-images/Dockerfile" {
		t.Fatalf("content %q", parts[0].Content)
	}
}

func TestWebImagePartsBracketedRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "[x]")
	writeFiles(t, root, ".lodev/web-images/Dockerfile")
	parts, err := WebImageParts(root)
	if err != nil || len(parts) != 1 {
		t.Fatalf("got %v, %v", parts, err)
	}
	if parts, err = WebImageParts(t.TempDir()); err != nil || len(parts) != 0 {
		t.Fatalf("missing dir: %v, %v", parts, err)
	}
}

func TestProjectHashNestedFile(t *testing.T) {
	ctx := t.TempDir()
	writeFiles(t, ctx, "conf/my.cnf")
	h := func(base string) string {
		s, err := projectHash([]byte("D"), base, ctx)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	h1 := h("B")
	if h1 != h("B") {
		t.Fatal("hash is not stable")
	}
	if h("B2") == h1 {
		t.Fatal("base ID must change the hash")
	}
	if err := os.WriteFile(filepath.Join(ctx, "conf/my.cnf"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h("B") == h1 {
		t.Fatal("nested file must change the hash")
	}
}

func TestEnsureProjectImage(t *testing.T) {
	cases := []struct {
		name     string
		nodejs   string
		webImgs  bool
		rebuild  bool
		existing bool // lodev/shop-web exists
		match    bool // its label matches the current hash
		wantTag  string
		wantCmds []string
		wantCtx  string // build context dir under root
	}{
		{"not needed, no image", "", false, false, false, false, "lodev/php", nil, ""},
		{"not needed, stale image", "", false, false, true, false, "lodev/php", nil, ""},
		{"needed, label matches", "20", false, false, true, true, "lodev/shop-web", nil, ""},
		{"needed, label differs", "20", true, false, true, false, "lodev/shop-web", []string{"build"}, ".lodev/web-images"},
		{"needed, no web-images", "20", false, false, false, false, "lodev/shop-web", []string{"build"}, ".lodev/web-build"},
		{"rebuild", "20", false, true, true, true, "lodev/shop-web", []string{"build --no-cache"}, ".lodev/web-build"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			p := &config.Project{Name: "shop", PHP: "8.3", NodeJS: c.nodejs, Root: root}
			if c.webImgs {
				writeFiles(t, root, ".lodev/web-images/conf/a")
			}
			labels := map[string]string{"lodev/php#id": "BASE"}
			fakeImages(t, labels)
			var cmds []string
			run = func(args ...string) error { cmds = append(cmds, strings.Join(args, " ")); return nil }
			if c.existing {
				labels["lodev/shop-web#id"] = "IMG"
				labels["lodev/shop-web"] = "stale"
			}
			if c.match {
				parts, _ := WebImageParts(root)
				df, _ := templates.WebBuildDockerfile(p, parts)
				h, _ := projectHash(df, "BASE", filepath.Join(root, ".lodev/web-images"))
				labels["lodev/shop-web"] = h
			}
			tag, err := EnsureProjectImage(p, c.rebuild)
			if err != nil {
				t.Fatal(err)
			}
			if tag != c.wantTag {
				t.Fatalf("tag %q, want %q", tag, c.wantTag)
			}
			if len(cmds) != len(c.wantCmds) {
				t.Fatalf("cmds %q, want %q", cmds, c.wantCmds)
			}
			for i, w := range c.wantCmds {
				if !strings.HasPrefix(cmds[i], w) {
					t.Fatalf("cmd %q, want prefix %q", cmds[i], w)
				}
			}
			if c.wantCtx != "" {
				df := filepath.Join(root, ".lodev/web-build/Dockerfile")
				if !strings.HasSuffix(cmds[0], "-f "+df+" "+filepath.Join(root, c.wantCtx)) {
					t.Fatalf("cmd %q: wrong -f/ctx", cmds[0])
				}
			}
			_, err = os.Stat(filepath.Join(root, ".lodev/web-build/Dockerfile"))
			if (err == nil) != (c.wantTag != "lodev/php") {
				t.Fatalf("web-build/Dockerfile presence wrong (err=%v)", err)
			}
		})
	}
}

func TestComposeConfigArgs(t *testing.T) {
	check(t, ComposeConfigArgs("P", []string{"a.yml", "b.yaml"}), "compose -p P -f a.yml -f b.yaml config")
}
