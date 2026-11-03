package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	if *update {
		os.WriteFile(name, got, 0o644)
	}
	want, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s mismatch:\n--- got\n%s--- want\n%s", name, got, want)
	}
}

const editSrc = "# project\nname: shop\nphp: \"8.3\"   # pinned\nservices: [mysql, redis]\ndomains:\n    - shop.test\n"

func TestDocSetKeepsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(editSrc), 0o600)
	d, err := OpenDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Set([]string{"php"}, "8.4")
	d.Set([]string{"nodejs"}, "24")
	d.Set([]string{"services"}, []string{"redis"})
	d.Set([]string{"domains"}, []string{"shop.test", "b.test"})
	d.Set([]string{"versions", "mysql"}, "8.0")
	d.Set([]string{"ports", "http"}, 8081)
	d.Set([]string{"services2", "x"}, true)
	d.Delete([]string{"name"})
	d.Delete([]string{"missing"})
	data, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "testdata/edit-project.golden", data)
	if err := d.Write(data); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	if b, _ := os.ReadFile(path); string(b) != string(data) {
		t.Fatalf("written %q", b)
	}
}

func TestDocEmptyAndCommentOnly(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"missing": "", "empty": "\x00", "comments": "# only\n"} {
		path := filepath.Join(dir, name, "c.yaml")
		if body != "" {
			os.MkdirAll(filepath.Dir(path), 0o755)
			os.WriteFile(path, []byte(strings.TrimPrefix(body, "\x00")), 0o644)
		}
		d, err := OpenDoc(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		d.Set([]string{"php"}, "8.4")
		data, _ := d.Bytes()
		if !strings.Contains(string(data), "php: \"8.4\"\n") {
			t.Fatalf("%s: %q", name, data)
		}
		if err := d.Write(data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if st, err := os.Stat(path); err != nil || (name == "missing" && st.Mode().Perm() != 0o644) {
			t.Fatalf("%s: %v %v", name, st, err)
		}
	}
}

func TestOpenDocErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	os.WriteFile(bad, []byte("a: ["), 0o644)
	if _, err := OpenDoc(bad); err == nil || !strings.HasPrefix(err.Error(), bad+": ") {
		t.Fatal(err)
	}
	list := filepath.Join(dir, "list.yaml")
	os.WriteFile(list, []byte("- a\n"), 0o644)
	if _, err := OpenDoc(list); err == nil || err.Error() != list+": not a YAML mapping" {
		t.Fatal(err)
	}
}
