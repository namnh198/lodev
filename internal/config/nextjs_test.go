package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func touch(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectNextjs(t *testing.T) {
	for _, f := range []string{"next.config.ts", "next.config.js", "next.config.mjs"} {
		root := t.TempDir()
		touch(t, root, f)
		if got := DetectType(root); got != "nextjs" {
			t.Errorf("%s: %s", f, got)
		}
	}
	root := t.TempDir()
	touch(t, root, "next.config.ts", "bin/magento")
	if got := DetectType(root); got != "magento" {
		t.Errorf("magento with next.config: %s", got)
	}
	root = t.TempDir()
	touch(t, root, "next.config.cjs")
	if got := DetectType(root); got != "php" {
		t.Errorf("next.config.cjs: %s", got)
	}
}

func TestNodePackageManager(t *testing.T) {
	for _, c := range []struct {
		files []string
		want  string
	}{
		{nil, "pnpm"},
		{[]string{"yarn.lock"}, "yarn"},
		{[]string{"package-lock.json"}, "npm"},
		{[]string{"pnpm-lock.yaml", "package-lock.json"}, "pnpm"},
		{[]string{"yarn.lock", "package-lock.json"}, "yarn"},
	} {
		root := t.TempDir()
		touch(t, root, c.files...)
		if got := (&Project{Root: root}).NodePackageManager(); got != c.want {
			t.Errorf("%v: %s, want %s", c.files, got, c.want)
		}
	}
}

func TestNextjsDaemons(t *testing.T) {
	for _, c := range []struct {
		lock, want string
	}{
		{"", `sh -c 'pnpm install && PORT=3000 exec pnpm run dev'`},
		{"pnpm-lock.yaml", `sh -c 'pnpm install --frozen-lockfile && PORT=3000 exec pnpm run dev'`},
		{"yarn.lock", `sh -c 'yarn install && PORT=3000 exec yarn run dev'`},
		{"package-lock.json", `sh -c 'npm ci && PORT=3000 exec npm run dev'`},
	} {
		root := t.TempDir()
		if c.lock != "" {
			touch(t, root, c.lock)
		}
		user := Daemon{Name: "worker", Command: "node worker.js"}
		p := &Project{Type: "nextjs", Root: root, WebExtraDaemons: []Daemon{user}}
		want := []Daemon{user, {Name: NextjsDaemon, Command: c.want}}
		if got := p.Daemons(); !slices.Equal(got, want) {
			t.Errorf("%q: %+v", c.lock, got)
		}
	}
	p := &Project{Type: "php", Root: t.TempDir(), WebExtraDaemons: []Daemon{{Name: "vite", Command: "npm run dev"}}}
	if got := p.Daemons(); !slices.Equal(got, p.WebExtraDaemons) {
		t.Errorf("php: %+v", got)
	}
}

func TestValidateNextjs(t *testing.T) {
	base := Project{Name: "x", Type: "nextjs", PHP: "8.4", Webserver: "nginx", Domains: []string{"x.test"}}
	if err := base.ValidateTLD("test"); err != nil {
		t.Fatalf("nginx: %v", err)
	}
	for _, ws := range []string{"apache", "none"} {
		p := base
		p.Webserver = ws
		if err := p.ValidateTLD("test"); err == nil || err.Error() != "nextjs needs webserver nginx (webserver is "+ws+")" {
			t.Errorf("%s: %v", ws, err)
		}
	}
	p := base
	p.WebExtraDaemons = []Daemon{{Name: "nextjs", Command: "x"}}
	if err := p.ValidateTLD("test"); err == nil || err.Error() != `web_extra_daemons name "nextjs" is reserved for type nextjs` {
		t.Errorf("reserved: %v", err)
	}
	p.Type = "php"
	if err := p.ValidateTLD("test"); err != nil {
		t.Errorf("php may use the name: %v", err)
	}
}

func TestNextjsVolumeDirs(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "package.json")
	p := &Project{Type: "nextjs", Root: root}
	want := []VolumeDir{{"node_modules", false}, {".next", false}}
	if got := p.VolumeDirsFor(&Global{PerformanceMode: "bind"}); !slices.Equal(got, want) {
		t.Errorf("bind: %+v", got)
	}
	if got := p.VolumeDirsFor(&Global{PerformanceMode: "none"}); got != nil {
		t.Errorf("none: %+v", got)
	}
}
