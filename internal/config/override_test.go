package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func writeLodev(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, ".lodev")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const overrideBase = "name: shop\ntype: php\nphp: \"8.3\"\nwebserver: nginx\ndocroot: public\nservices: [mysql]\n"

func TestOverrideFilesSorted(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"config.b.yaml", "config.a.yaml", "config.yaml", "config.x.yml"} {
		writeLodev(t, root, n, "")
	}
	if err := os.Mkdir(filepath.Join(root, ".lodev", "config.d.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := OverrideFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, ".lodev", "config.a.yaml"), filepath.Join(root, ".lodev", "config.b.yaml")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
}

func TestMergeOverrides(t *testing.T) {
	root := t.TempDir()
	writeLodev(t, root, "config.local.yaml", "php: \"8.4\"\nenvironment: {B: \"3\"}\ndomains: [c.test]\nhooks: {post-start: [z]}\n")
	base := "php: \"8.3\"\nenvironment: {A: \"1\", B: \"2\"}\ndomains: [a.test, b.test]\nhooks: {post-start: [x], pre-stop: [y]}\n"
	merged, files, err := MergeOverrides(root, []byte(base))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatal(files)
	}
	var got map[string]any
	if err := yaml.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"php":         "8.4",
		"environment": map[string]any{"A": "1", "B": "3"},
		"domains":     []any{"c.test"},
		"hooks":       map[string]any{"post-start": []any{"z"}, "pre-stop": []any{"y"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
}

func TestMergeOverridesRejectsName(t *testing.T) {
	root := t.TempDir()
	writeLodev(t, root, "config.local.yaml", "name: other\n")
	_, _, err := MergeOverrides(root, []byte(overrideBase))
	if err == nil || err.Error() != ".lodev/config.local.yaml: name cannot be overridden" {
		t.Fatal(err)
	}
}

func TestMergeOverridesBadYAML(t *testing.T) {
	root := t.TempDir()
	writeLodev(t, root, "config.local.yaml", "php: [\n")
	_, _, err := MergeOverrides(root, []byte(overrideBase))
	if err == nil || !strings.HasPrefix(err.Error(), ".lodev/config.local.yaml:") {
		t.Fatal(err)
	}
}

func TestMergeOverridesEmptyFile(t *testing.T) {
	root := t.TempDir()
	writeLodev(t, root, "config.local.yaml", "# nothing yet\n")
	merged, _, err := MergeOverrides(root, []byte(overrideBase))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseProject(root, merged); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectAppliesOverrides(t *testing.T) {
	root := t.TempDir()
	writeLodev(t, root, "config.yaml", overrideBase)
	writeLodev(t, root, "config.local.yaml", "php: \"8.4\"\n")
	p, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if p.PHP != "8.4" {
		t.Fatal(p.PHP)
	}
	writeLodev(t, root, "config.local.yaml", "php: \"5.6\"\n")
	_, err = LoadProject(root)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("%T %v", err, err)
	}
}

func TestOverriddenKeys(t *testing.T) {
	root := t.TempDir()
	writeLodev(t, root, "config.a.yaml", "php: \"8.4\"\n")
	writeLodev(t, root, "config.b.yaml", "php: \"8.2\"\n")
	got, err := OverriddenKeys(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"php": "config.b.yaml"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%v want %v", got, want)
	}
}
