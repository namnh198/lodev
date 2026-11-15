package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
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
	if err != nil || !strings.HasSuffix(string(b), "exec sh -c 'pnpm install && PORT=3000 exec pnpm run dev'\n") {
		t.Fatalf("nextjs.sh: %q %v", b, err)
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
	writeFile(t, filepath.Join(p.Root, "package.json"), `{"scripts":{"dev":"next dev --turbopack"}}`)
	if got, want := display(p), `nextjs: pnpm install && pnpm run dev (package.json dev: "next dev --turbopack")`; got != want {
		t.Errorf("display\n got %s\nwant %s", got, want)
	}
	for name, body := range map[string]string{"no dev script": `{"scripts":{}}`, "invalid JSON": `{`} {
		writeFile(t, filepath.Join(p.Root, "package.json"), body)
		if got := display(p); !strings.HasSuffix(got, "(package.json has no dev script)") {
			t.Errorf("%s: %s", name, got)
		}
	}
	os.Remove(filepath.Join(p.Root, "package.json"))
	if got := display(p); !strings.HasSuffix(got, "(package.json has no dev script)") {
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
	if w := warnings(t, p, "none"); !strings.Contains(w, "node_modules") || !strings.Contains(w, "lodev config --performance-mode=bind") {
		t.Errorf("mode none: %q", w)
	}
	if w := warnings(t, p, "bind"); strings.Contains(w, "node_modules") {
		t.Errorf("mode bind: %q", w)
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
