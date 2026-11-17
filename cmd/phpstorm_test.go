package cmd

import (
	"regexp"
	"strings"
	"testing"
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
