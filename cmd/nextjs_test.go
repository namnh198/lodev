package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

// nextjsConfProject is a nextjs project in a temp dir, not registered.
func nextjsConfProject(t *testing.T) *config.Project {
	t.Helper()
	p, err := newProjectConfig(t.TempDir(), "web", "nextjs", "8.4", "nginx", "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWriteDaemonsNextjs(t *testing.T) {
	p := nextjsConfProject(t)
	if err := writeDaemons(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p.Root, ".lodev", "daemons", "nextjs.sh"))
	if err != nil || !strings.HasSuffix(string(b), "exec "+p.Daemons()[0].Command+"\n") || !strings.Contains(string(b), "run build") {
		t.Fatalf("nextjs.sh: %q %v", b, err)
	}
}

// A nextjs.sh the user took over (marker line deleted) is kept, said so, tracked by git and listed as a repo command.
func TestWriteDaemonsKeepsTakenOverNextjs(t *testing.T) {
	p := nextjsConfProject(t)
	f := filepath.Join(p.Root, ".lodev", "daemons", "nextjs.sh")
	writeFile(t, f, "#!/bin/bash\nexec bun dev\n")
	o, _, restore := ui.Capture(false)
	err := writeDaemons(p)
	restore()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "#!/bin/bash\nexec bun dev\n" {
		t.Fatalf("taken-over nextjs.sh rewritten: %q", b)
	}
	if !strings.Contains(o.String(), ".lodev/daemons/nextjs.sh is user-managed; not regenerating") {
		t.Errorf("no message: %q", o.String())
	}
	if _, err := os.Stat(filepath.Join(p.Root, ".lodev", "daemons", "daemons.conf")); err != nil {
		t.Errorf("daemons.conf: %v", err)
	}
	if err := writeLodevGitignore(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(p.Root, ".lodev", ".gitignore")); !strings.Contains(string(b), "/daemons/*\n!/daemons/nextjs.sh\n") {
		t.Errorf("nextjs.sh still ignored: %q", b)
	}
	items, err := trustItems(p)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(items, func(it trustItem) bool { return it.Key == "nextjs" }); i < 0 || items[i].Display != "nextjs: .lodev/daemons/nextjs.sh" {
		t.Errorf("trust items: %v", items)
	}
	// The project stops being nextjs: lodev still never deletes the user's script.
	p.Type, p.Webserver = "php", "nginx"
	if err := writeDaemons(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); err != nil {
		t.Errorf("user's nextjs.sh deleted: %v", err)
	}
}

// An older lodev wrote nextjs.sh without the marker: it is still lodev's, so it gets regenerated.
func TestWriteDaemonsUpgradesUnmarkedNextjs(t *testing.T) {
	p := nextjsConfProject(t)
	f := filepath.Join(p.Root, ".lodev", "daemons", "nextjs.sh")
	writeFile(t, f, "#!/bin/bash\ncd '/app' || exit 1\nexec sh -c 'pnpm install && PORT=3000 exec pnpm run dev'\n")
	if err := writeDaemons(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); !strings.HasPrefix(string(b), templates.GeneratedMarker+"\n") {
		t.Fatalf("old nextjs.sh not regenerated: %q", b)
	}
}

// A conf the user took over (marker line deleted) stays theirs on a nextjs project too.
func TestWriteWebserverConfKeepsTakenOverNextjs(t *testing.T) {
	p := nextjsConfProject(t)
	f := filepath.Join(p.Root, ".lodev", "nginx-webserver.conf")
	writeFile(t, f, "server { listen 80; }\n")
	if err := writeWebserverConf(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "server { listen 80; }\n" {
		t.Fatalf("taken-over conf rewritten: %q", b)
	}
}

func TestNewProjectConfigDetectsNextjs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "next.config.ts"), "export default {}\n")
	p, err := newProjectConfig(root, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != "nextjs" || len(p.Services) != 0 || p.Webserver != "nginx" || p.PHP != "8.4" {
		t.Fatalf("%+v", p)
	}
}

func TestAskProjectNextjsSkipsPHPAndDocroot(t *testing.T) {
	p := newAskProject(t)
	var out bytes.Buffer
	if err := askProject(strings.NewReader("\nnextjs\n\n"), &out, p, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if p.Type != "nextjs" || p.PHP != "8.4" || p.Docroot != "" || p.Webserver != "nginx" || len(p.Services) != 0 {
		t.Fatalf("%+v", p)
	}
	if s := out.String(); strings.Contains(s, "PHP version") || strings.Contains(s, "Document root") {
		t.Errorf("asked PHP or docroot:\n%s", s)
	}
}

func TestTrustItemsNextjs(t *testing.T) {
	display := func(p *config.Project) string {
		t.Helper()
		items, err := trustItems(p)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(items, func(it trustItem) bool { return it.Key == "nextjs" })
		if i < 0 {
			t.Fatalf("no nextjs item: %v", items)
		}
		return items[i].Display
	}
	p := nextjsConfProject(t)
	writeFile(t, filepath.Join(p.Root, "package.json"), `{"scripts":{"dev":"next dev --turbopack","build":"next build","start":"next start"}}`)
	if got, want := display(p), `nextjs: pnpm install && pnpm run dev, or with NODE_ENV=production pnpm run build && pnpm run start (package.json dev: "next dev --turbopack", build: "next build", start: "next start")`; got != want {
		t.Errorf("display\n got %s\nwant %s", got, want)
	}
	writeFile(t, filepath.Join(p.Root, "package.json"), `{"scripts":{"preview":"vite preview"}}`)
	if got, want := display(p), `nextjs: pnpm install && pnpm run dev, or with NODE_ENV=production pnpm run build && pnpm run preview (package.json dev: none, build: none, preview: "vite preview")`; got != want {
		t.Errorf("preview\n got %s\nwant %s", got, want)
	}
	const none = "(package.json dev: none, build: none, start: none)"
	for name, body := range map[string]string{"no scripts": `{"scripts":{}}`, "invalid JSON": `{`} {
		writeFile(t, filepath.Join(p.Root, "package.json"), body)
		if got := display(p); !strings.HasSuffix(got, none) {
			t.Errorf("%s: %s", name, got)
		}
	}
	os.Remove(filepath.Join(p.Root, "package.json"))
	if got := display(p); !strings.HasSuffix(got, none) {
		t.Errorf("no package.json: %s", got)
	}
}

// warnings is what warnNextjs prints.
func warnings(t *testing.T, p *config.Project, mode string) string {
	t.Helper()
	o, e, restore := ui.Capture(false)
	defer restore()
	warnNextjs(p, &config.Global{PerformanceMode: mode})
	return o.String() + e.String()
}

func TestWarnNextjsHostNodeModules(t *testing.T) {
	p := nextjsConfProject(t)
	os.MkdirAll(filepath.Join(p.Root, "node_modules"), 0o755)
	defer func(o string) { setupOS = o }(setupOS)
	setupOS = "darwin"
	want := "warning: performance_mode is none: the container installs Linux packages into the host's node_modules; run lodev config --performance-mode=bind to keep them in a volume\n"
	if w := warnings(t, p, "none"); w != want {
		t.Errorf("mode none, node_modules:\n got %q\nwant %q", w, want)
	}
	// Both folders: one warning naming them.
	os.MkdirAll(filepath.Join(p.Root, ".next"), 0o755)
	want = "warning: performance_mode is none: the container installs Linux packages into the host's node_modules, .next; run lodev config --performance-mode=bind to keep them in a volume\n"
	if w := warnings(t, p, "none"); w != want {
		t.Errorf("mode none, both:\n got %q\nwant %q", w, want)
	}
	if w := warnings(t, p, "bind"); strings.Contains(w, "node_modules") {
		t.Errorf("mode bind: %q", w)
	}
	// A Linux host runs the same Linux packages, so its node_modules is fine.
	setupOS = "linux"
	if w := warnings(t, p, "none"); strings.Contains(w, "node_modules") {
		t.Errorf("linux host, mode none: %q", w)
	}
}

func TestWarnNextjsAllowedDevOrigins(t *testing.T) {
	p := nextjsConfProject(t)
	if w := warnings(t, p, "bind"); w != "" {
		t.Errorf("no next.config: %q", w)
	}
	conf := filepath.Join(p.Root, "next.config.ts")
	writeFile(t, conf, "export default { cacheComponents: true }\n")
	if w := warnings(t, p, "bind"); !strings.Contains(w, `allowedDevOrigins: ["web.test"]`) {
		t.Errorf("missing domain: %q", w)
	}
	writeFile(t, conf, "export default { allowedDevOrigins: [\"web.test\"] }\n")
	if w := warnings(t, p, "bind"); w != "" {
		t.Errorf("domain present: %q", w)
	}
	// Only a nextjs project is checked.
	p.Type = "php"
	os.Remove(conf)
	writeFile(t, conf, "export default {}\n")
	if w := warnings(t, p, "bind"); w != "" {
		t.Errorf("php project: %q", w)
	}
}

// The nextjs daemon installs with the lockfile's manager: pointing at npm would switch a pnpm project to npm.
func TestEnsureVolumesHintNextjs(t *testing.T) {
	p, g, _ := volumeFixture(t, missing)
	p.Type, p.VolumeDirs = "nextjs", nil
	writeFile(t, filepath.Join(p.Root, "package.json"), "{}")
	out, _, restore := ui.Capture(false)
	defer restore()
	if _, err := ensureVolumes(p, g, "img"); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if strings.Contains(s, "lodev npm install") {
		t.Errorf("nextjs hint points at npm: %s", s)
	}
	mustContain(t, s, "daemons retry on their own")
}
