package commands

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"lodev/internal/templates"
)

func TestFilterSuggestions(t *testing.T) {
	out := "install\nrequire\tAdd a package\nremove\n\nreinstall\r\n"
	if got := FilterSuggestions(out, "re"); !slices.Equal(got, []string{"require\tAdd a package", "remove", "reinstall"}) {
		t.Errorf("prefix re: %q", got)
	}
	if got := FilterSuggestions(out, ""); len(got) != 4 {
		t.Errorf("empty word: %q", got)
	}
	if got := FilterSuggestions(out, "Add"); got != nil {
		t.Errorf("a description matched: %q", got)
	}
	if got := FilterSuggestions("", ""); got != nil {
		t.Errorf("empty output: %q", got)
	}
}

func TestLoadDirAutocomplete(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a", "echo a\n")
	write("b", "echo b\n")
	write("autocomplete/a", "echo one\n")
	cmds, w := LoadDir(dir, Host)
	if len(cmds) != 2 || len(w) != 0 {
		t.Fatalf("cmds %+v, warnings %q", cmds, w)
	}
	ac := cmds[0].Complete
	if ac == nil || ac.Script != "echo one\n" || ac.Kind != Host || ac.Path != filepath.Join(dir, "autocomplete", "a") {
		t.Errorf("a.Complete = %+v", ac)
	}
	if cmds[1].Complete != nil {
		t.Errorf("b.Complete = %+v", cmds[1].Complete)
	}
}

// A global command keeps its own autocomplete, but lodev's synced (marker) copy is dropped.
func TestResolveGlobalAutocomplete(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "commands", Web)
	os.MkdirAll(filepath.Join(dir, "autocomplete"), 0o755)
	os.WriteFile(filepath.Join(dir, "mine"), []byte("echo mine\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "autocomplete", "mine"), []byte("echo m\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "composer"), []byte("echo mine\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "autocomplete", "composer"), []byte(templates.GeneratedMarker+"\necho synced\n"), 0o644)
	cmds, _ := Resolve(nil, home, "", "", false, nil)
	if len(cmds) != 2 || cmds[0].Name != "composer" || cmds[0].Complete != nil {
		t.Fatalf("composer: %+v", cmds)
	}
	if cmds[1].Complete == nil || cmds[1].Complete.Script != "echo m\n" {
		t.Errorf("mine: %+v", cmds[1])
	}
}

func withAssets(t *testing.T, m fstest.MapFS) {
	t.Helper()
	old := assets
	t.Cleanup(func() { assets = old })
	assets = m
}

func TestShippedAndSyncAutocomplete(t *testing.T) {
	ac := templates.GeneratedMarker + "\necho install\n"
	withAssets(t, fstest.MapFS{
		"commands/web/composer":              {Data: []byte(templates.GeneratedMarker + "\nexec composer \"$@\"\n")},
		"commands/web/autocomplete/composer": {Data: []byte(ac)},
		"commands/web/npm":                   {Data: []byte(templates.GeneratedMarker + "\nexec npm \"$@\"\n")},
	})
	cmds := Shipped()
	if len(cmds) != 2 || cmds[0].Name != "composer" || cmds[1].Name != "npm" {
		t.Fatalf("shipped = %+v", cmds)
	}
	if c := cmds[0].Complete; c == nil || c.Script != ac || !c.Marker || c.Path != "" || c.Kind != Web {
		t.Errorf("composer.Complete = %+v", c)
	}
	if cmds[1].Complete != nil {
		t.Errorf("npm.Complete = %+v", cmds[1].Complete)
	}

	home := t.TempDir()
	p := filepath.Join(home, "commands", Web, "autocomplete", "composer")
	if w := Sync(home); len(w) != 0 {
		t.Fatalf("warnings %q", w)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != ac {
		t.Errorf("synced autocomplete: %q, %v", b, err)
	}
	os.WriteFile(p, []byte(templates.GeneratedMarker+"\nold\n"), 0o644)
	Sync(home)
	if b, _ := os.ReadFile(p); string(b) != ac {
		t.Errorf("marker autocomplete not rewritten: %q", b)
	}
	os.WriteFile(p, []byte("echo mine\n"), 0o644)
	Sync(home)
	if b, _ := os.ReadFile(p); string(b) != "echo mine\n" {
		t.Errorf("user autocomplete touched: %q", b)
	}
}
