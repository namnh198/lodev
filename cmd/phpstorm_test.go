package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// Trimmed from a PhpStorm 2026.x .idea/php.xml with a hand-made Docker Compose interpreter.
const phpXMLFixture = `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  <component name="LaravelPint">
    <laravel_pint_settings>
      <LaravelPintConfiguration tool_path="$PROJECT_DIR$/vendor/bin/pint" />
    </laravel_pint_settings>
  </component>
  <component name="PhpInterpreters">
    <interpreters>
      <interpreter id="77779de8-1c2a-45c9-b5fe-739cc70d1d7f" name="web" home="docker-compose://DATA" auto="false" debugger_id="php.debugger.XDebug">
        <remote_data INTERPRETER_PATH="/usr/bin/php8.4" DOCKER_COMPOSE_SERVICE_NAME="web">
          <type_data command="EXEC" />
        </remote_data>
      </interpreter>
    </interpreters>
  </component>
  <component name="PhpInterpretersPhpInfoCache">
    <phpInfoCache>
      <interpreter name="web">
        <phpinfo binary_type="PHP" php_cli="/usr/bin/php8.4" version="8.4.27" />
      </interpreter>
    </phpInfoCache>
  </component>
</project>
`

// Trimmed from the same project's .idea/workspace.xml.
const workspaceXMLFixture = `<?xml version="1.0" encoding="UTF-8"?>
<project version="4">
  <component name="PhpDebugGeneral" listening_started="true" />
  <component name="PhpServers">
    <servers>
      <server host="laravel.test" id="1477fc9e-93b6-4ea6-bad7-32ee68bb7bd2" name="laravel" use_path_mappings="true">
        <path_mappings>
          <mapping local-root="$PROJECT_DIR$" remote-root="/var/www/html" />
        </path_mappings>
      </server>
    </servers>
  </component>
  <component name="PhpWorkspaceProjectConfiguration" interpreter_name="web">
    <include_path>
      <path value="$PROJECT_DIR$/vendor/monolog/monolog" />
    </include_path>
  </component>
</project>
`

const (
	fixtureWebInterpreter = `      <interpreter id="77779de8-1c2a-45c9-b5fe-739cc70d1d7f" name="web" home="docker-compose://DATA" auto="false" debugger_id="php.debugger.XDebug">
        <remote_data INTERPRETER_PATH="/usr/bin/php8.4" DOCKER_COMPOSE_SERVICE_NAME="web">
          <type_data command="EXEC" />
        </remote_data>
      </interpreter>
`
	fixtureInfoCache = `  <component name="PhpInterpretersPhpInfoCache">
    <phpInfoCache>
      <interpreter name="web">
        <phpinfo binary_type="PHP" php_cli="/usr/bin/php8.4" version="8.4.27" />
      </interpreter>
    </phpInfoCache>
  </component>
`
)

// fakeInterpreter builds a one-line lodev interpreter carrying id and a body marker.
func fakeInterpreter(body string) func(string) string {
	return func(id string) string {
		return `      <interpreter id="` + id + `" name="lodev">` + body + "</interpreter>\n"
	}
}

func upsertLodev(doc, body string) string {
	return upsertElement(doc, "PhpInterpreters", "interpreters", "interpreter", "lodev", fakeInterpreter(body))
}

func TestUpsertElementAppends(t *testing.T) {
	out := upsertLodev(phpXMLFixture, "NEW")
	mustContain(t, out, fixtureWebInterpreter, fixtureInfoCache, `name="LaravelPint"`)
	if n := strings.Count(out, `name="lodev"`); n != 1 {
		t.Fatalf("lodev interpreters = %d, want 1:\n%s", n, out)
	}
	// It lands inside PhpInterpreters, after web, before the list closes.
	web := strings.Index(out, fixtureWebInterpreter)
	lodev := strings.Index(out, `name="lodev"`)
	end := strings.Index(out, "    </interpreters>")
	if !(web < lodev && lodev < end) {
		t.Fatalf("lodev not appended inside <interpreters>:\n%s", out)
	}
	if strings.Contains(fixtureInfoCache, "lodev") || !strings.Contains(out, fixtureInfoCache) {
		t.Fatalf("info cache changed:\n%s", out)
	}
}

func TestUpsertElementReplacesKeepingID(t *testing.T) {
	doc := upsertElement(phpXMLFixture, "PhpInterpreters", "interpreters", "interpreter", "lodev",
		func(string) string { return `      <interpreter id="abc" name="lodev">OLD</interpreter>` + "\n" })
	var gotID string
	out := upsertElement(doc, "PhpInterpreters", "interpreters", "interpreter", "lodev", func(id string) string {
		gotID = id
		return fakeInterpreter("NEW")(id)
	})
	if gotID != "abc" {
		t.Fatalf("build got id %q, want abc", gotID)
	}
	if strings.Contains(out, "OLD") || strings.Count(out, `name="lodev"`) != 1 {
		t.Fatalf("old lodev interpreter not replaced:\n%s", out)
	}
	mustContain(t, out, `<interpreter id="abc" name="lodev">NEW</interpreter>`, fixtureWebInterpreter, fixtureInfoCache)
}

func TestUpsertElementReplacesMultiline(t *testing.T) {
	out := upsertElement(workspaceXMLFixture, "PhpServers", "servers", "server", "laravel",
		func(id string) string { return `      <server id="` + id + `" name="laravel" />` + "\n" })
	if strings.Contains(out, "/var/www/html") {
		t.Fatalf("old server body kept:\n%s", out)
	}
	mustContain(t, out, `      <server id="1477fc9e-93b6-4ea6-bad7-32ee68bb7bd2" name="laravel" />`+"\n    </servers>", `interpreter_name="web"`)
}

func TestUpsertElementIdempotent(t *testing.T) {
	once := upsertLodev(phpXMLFixture, "NEW")
	if twice := upsertLodev(once, "NEW"); twice != once {
		t.Fatalf("second run changed the file:\n%s\n---\n%s", once, twice)
	}
}

func TestUpsertElementCreatesComponent(t *testing.T) {
	out := upsertLodev(ideaSkeleton, "NEW")
	want := "<project version=\"4\">\n  <component name=\"PhpInterpreters\">\n    <interpreters>\n      <interpreter id=\""
	mustContain(t, out, want, "\">NEW</interpreter>\n    </interpreters>\n  </component>\n</project>\n")

	selfClosing := "<project version=\"4\">\n  <component name=\"PhpInterpreters\" />\n</project>\n"
	out = upsertLodev(selfClosing, "NEW")
	mustContain(t, out, "  <component name=\"PhpInterpreters\">\n    <interpreters>\n      <interpreter id=\"", "</interpreters>\n  </component>\n</project>\n")
	if strings.Contains(out, "/>\n</project>") {
		t.Fatalf("self-closing component left:\n%s", out)
	}
}

func TestSetComponentAttr(t *testing.T) {
	out := setComponentAttr(workspaceXMLFixture, "PhpWorkspaceProjectConfiguration", "interpreter_name", "lodev")
	mustContain(t, out, `<component name="PhpWorkspaceProjectConfiguration" interpreter_name="lodev">`, `<path value="$PROJECT_DIR$/vendor/monolog/monolog" />`)

	out = setComponentAttr("<project version=\"4\">\n  <component name=\"X\">\n  </component>\n</project>\n", "X", "a", "b")
	mustContain(t, out, `<component name="X" a="b">`)

	out = setComponentAttr(ideaSkeleton, "PhpWorkspaceProjectConfiguration", "interpreter_name", "lodev")
	mustContain(t, out, "<project version=\"4\">\n  <component name=\"PhpWorkspaceProjectConfiguration\" interpreter_name=\"lodev\" />\n</project>\n")

	if again := setComponentAttr(out, "PhpWorkspaceProjectConfiguration", "interpreter_name", "lodev"); again != out {
		t.Fatalf("second run changed the file:\n%s", again)
	}
}

func TestXMLAttr(t *testing.T) {
	if got := xmlAttr(`a&b<"c>`); got != `a&amp;b&lt;&quot;c&gt;` {
		t.Fatalf("xmlAttr = %q", got)
	}
}

func TestNewUUID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a, b := newUUID(), newUUID()
	if !re.MatchString(a) || a == b {
		t.Fatalf("newUUID = %q, %q", a, b)
	}
}

// phpstormProject is a started php project (its compose file exists); PhpStorm is not running.
func phpstormProject(t *testing.T) *config.Project {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	p := &config.Project{Name: "shop", Type: "php", PHP: "8.4", Domains: []string{"shop.test"}, Root: t.TempDir()}
	writeFile(t, composeFull(p), "services: {}\n")
	old := phpstormRunning
	t.Cleanup(func() { phpstormRunning = old })
	phpstormRunning = func() bool { return false }
	return p
}

func ideaText(t *testing.T, p *config.Project, file string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(p.Root, ".idea", file))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPhpstormFreshProject(t *testing.T) {
	p := phpstormProject(t)
	_, _, restore := ui.Capture(false)
	defer restore()
	if err := writePhpstorm(p, false); err != nil {
		t.Fatal(err)
	}
	mustContain(t, ideaText(t, p, "php.xml"),
		`<component name="PhpInterpreters">`, `name="lodev" home="docker-compose://DATA"`,
		`INTERPRETER_PATH="/usr/bin/php8.4"`, `DOCKER_COMPOSE_SERVICE_NAME="web"`, `<type_data command="EXEC" />`,
		`<item value="$PROJECT_DIR$/.lodev/lodev.compose-full.yml" />`, `debugger_id="php.debugger.XDebug"`)
	mustContain(t, ideaText(t, p, "workspace.xml"),
		`<server host="shop.test" id="`, `name="shop" use_path_mappings="true"`,
		`<mapping local-root="$PROJECT_DIR$" remote-root="/app" />`,
		`<component name="PhpWorkspaceProjectConfiguration" interpreter_name="lodev" />`)
}

func TestPhpstormKeepsUserSettings(t *testing.T) {
	p := phpstormProject(t)
	writeFile(t, filepath.Join(p.Root, ".idea", "php.xml"), phpXMLFixture)
	writeFile(t, filepath.Join(p.Root, ".idea", "workspace.xml"), workspaceXMLFixture)
	_, _, restore := ui.Capture(false)
	defer restore()
	if err := writePhpstorm(p, false); err != nil {
		t.Fatal(err)
	}
	php, ws := ideaText(t, p, "php.xml"), ideaText(t, p, "workspace.xml")
	mustContain(t, php, fixtureWebInterpreter, fixtureInfoCache, `name="LaravelPint"`, `name="lodev"`)
	mustContain(t, ws, `name="laravel"`, `remote-root="/var/www/html"`, `<component name="PhpDebugGeneral" listening_started="true" />`,
		`<component name="PhpWorkspaceProjectConfiguration" interpreter_name="lodev">`, `vendor/monolog/monolog`, `name="shop"`)

	if err := writePhpstorm(p, false); err != nil {
		t.Fatal(err)
	}
	if ideaText(t, p, "php.xml") != php || ideaText(t, p, "workspace.xml") != ws {
		t.Fatal("second run changed .idea")
	}
}

func TestPhpstormUsesConfig(t *testing.T) {
	p := phpstormProject(t)
	p.PHPIDEConfig, p.WorkingDir, p.PHP = "myshop", "/var/www", "8.2"
	_, _, restore := ui.Capture(false)
	defer restore()
	if err := writePhpstorm(p, false); err != nil {
		t.Fatal(err)
	}
	mustContain(t, ideaText(t, p, "workspace.xml"), `name="myshop"`, `remote-root="/var/www"`)
	mustContain(t, ideaText(t, p, "php.xml"), `INTERPRETER_PATH="/usr/bin/php8.2"`)
}

func TestPhpstormNoCompose(t *testing.T) {
	p := phpstormProject(t)
	os.Remove(composeFull(p))
	err := writePhpstorm(p, false)
	if err == nil || err.Error() != ".lodev/lodev.compose-full.yml is missing: run lodev start first" {
		t.Fatalf("err = %v", err)
	}
}

func TestPhpstormNextjs(t *testing.T) {
	p := phpstormProject(t)
	p.Type = "nextjs"
	err := writePhpstorm(p, false)
	if err == nil || err.Error() != "nextjs projects have no php interpreter for phpstorm" {
		t.Fatalf("err = %v", err)
	}
}

func TestPhpstormRunningDeclined(t *testing.T) {
	p := phpstormProject(t)
	phpstormRunning = func() bool { return true }
	asTTY(t, true, "n")
	_, e, restore := ui.Capture(false)
	defer restore()
	err := writePhpstorm(p, false)
	if err == nil || err.Error() != "phpstorm is running: close the project first, or pass --yes" {
		t.Fatalf("err = %v", err)
	}
	mustContain(t, e.String(), "PhpStorm is running and may overwrite .idea/workspace.xml when it saves")
	if _, err := os.Stat(filepath.Join(p.Root, ".idea")); !os.IsNotExist(err) {
		t.Fatalf(".idea written after a no: %v", err)
	}
	if err := writePhpstorm(p, true); err != nil {
		t.Fatal(err)
	}
	mustContain(t, ideaText(t, p, "php.xml"), `name="lodev"`)
}

func TestPhpstormCommand(t *testing.T) {
	p := phpstormProject(t)
	writeFile(t, config.ProjectPath(p.Root), "name: shop\ntype: php\nphp: \"8.4\"\n")
	t.Chdir(p.Root)
	_, _, restore := ui.Capture(false)
	defer restore()
	if err := runRoot(t, "phpstorm"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, ideaText(t, p, "workspace.xml"), `host="shop.test"`)
}

func TestUpsertElementEmptyComponent(t *testing.T) {
	doc := "<project version=\"4\">\n  <component name=\"PhpServers\"></component>\n</project>\n"
	out := upsertElement(doc, "PhpServers", "servers", "server", "shop", func(id string) string { return "      <server name=\"shop\" />\n" })
	if strings.Contains(out, ">>") || strings.Count(out, "</component>") != 1 {
		t.Fatalf("empty component mangled:\n%s", out)
	}
	mustContain(t, out, "<servers>\n      <server name=\"shop\" />\n    </servers>")
}

func TestPhpstormEmptyFileIsFresh(t *testing.T) {
	p := phpstormProject(t)
	writeFile(t, filepath.Join(p.Root, ".idea", "workspace.xml"), "")
	_, _, restore := ui.Capture(false)
	defer restore()
	if err := writePhpstorm(p, false); err != nil {
		t.Fatal(err)
	}
	mustContain(t, ideaText(t, p, "workspace.xml"), `name="shop"`, `interpreter_name="lodev"`)
}

func TestPhpstormBrokenFile(t *testing.T) {
	for _, broken := range []string{
		"<project version=\"4\">\n  <component name=\"PhpServers\">\n    <servers>\n",
		"<project version=\"4\" />\n",
		"<project version=\"4\">\n  <component name=\"PhpServers\"\n",
		"<project version=\"4\">\n  <component name=\"PhpServers\">\n    <servers>\n      <server name=\"shop\">\n    </servers>\n  </component>\n</project>\n",
	} {
		p := phpstormProject(t)
		writeFile(t, filepath.Join(p.Root, ".idea", "workspace.xml"), broken)
		err := writePhpstorm(p, false)
		if err == nil || err.Error() != ".idea/workspace.xml is not a complete PhpStorm file: fix or delete it, then run lodev phpstorm again" {
			t.Fatalf("%q: err = %v", broken, err)
		}
		if _, err := os.Stat(filepath.Join(p.Root, ".idea", "php.xml")); !os.IsNotExist(err) {
			t.Fatalf("%q: php.xml written although workspace.xml is broken", broken)
		}
		if got := ideaText(t, p, "workspace.xml"); got != broken {
			t.Fatalf("broken file changed: %q", got)
		}
	}
}
