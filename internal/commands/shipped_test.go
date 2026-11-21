package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"lodev/internal/templates"
)

func TestSyncLeavesUnreadableAndSymlinkedFiles(t *testing.T) {
	home := t.TempDir()
	Sync(home)
	dir := filepath.Join(home, "commands", Web)
	locked, link, same := filepath.Join(dir, "php"), filepath.Join(dir, "composer"), filepath.Join(dir, "npm")
	os.WriteFile(locked, []byte("echo mine\n"), 0o644)
	os.Chmod(locked, 0o000)
	t.Cleanup(func() { os.Chmod(locked, 0o644) })
	if _, err := os.ReadFile(locked); err == nil {
		t.Skip("file permissions are not enforced (root)")
	}
	target := filepath.Join(home, "mine.sh")
	os.WriteFile(target, []byte(templates.GeneratedMarker+"\nold\n"), 0o644)
	os.Remove(link)
	os.Symlink(target, link)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	os.Chtimes(same, old, old)

	Sync(home)
	os.Chmod(locked, 0o644)
	if b, _ := os.ReadFile(locked); string(b) != "echo mine\n" {
		t.Errorf("unreadable user file replaced: %q", b)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink replaced: %v, %v", fi, err)
	}
	if b, _ := os.ReadFile(target); string(b) != templates.GeneratedMarker+"\nold\n" {
		t.Errorf("symlink target rewritten: %q", b)
	}
	if fi, _ := os.Stat(same); !fi.ModTime().Equal(old) {
		t.Errorf("identical marker file rewritten: mtime %v", fi.ModTime())
	}
}

func TestShippedWeb(t *testing.T) {
	want := []string{"php", "composer", "npm", "node", "pnpm", "yarn", "grunt", "artisan", "wp", "magento", "n98"}
	types := map[string][]string{"artisan": {"laravel"}, "wp": {"wordpress"}, "magento": {"magento"}, "n98": {"magento"}}
	var got []string
	for _, c := range Shipped() {
		if c.Kind != Web {
			continue
		}
		got = append(got, c.Name)
		parsed, err := Parse(c.Name, c.Kind, "", []byte(c.Script))
		if err != nil || !c.Marker || c.Path != "" || !reflect.DeepEqual(parsed.Types, types[c.Name]) || !reflect.DeepEqual(c.Types, types[c.Name]) {
			t.Errorf("%s: %+v, %v", c.Name, c, err)
		}
		if c.Name == "magento" && !strings.HasSuffix(c.Script, "exec php bin/magento \"$@\"\n") {
			t.Errorf("magento script: %q", c.Script)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("web commands %q, want %q", got, want)
	}
}

func TestSync(t *testing.T) {
	home := t.TempDir()
	if w := Sync(home); len(w) != 0 {
		t.Fatalf("warnings %q", w)
	}
	for _, c := range Shipped() {
		b, err := os.ReadFile(filepath.Join(home, "commands", c.Kind, c.Name))
		if err != nil || string(b) != c.Script {
			t.Errorf("%s/%s: %q, %v", c.Kind, c.Name, b, err)
		}
	}
	php, composer := filepath.Join(home, "commands", Web, "php"), filepath.Join(home, "commands", Web, "composer")
	os.WriteFile(php, []byte(templates.GeneratedMarker+"\nold\n"), 0o644)
	os.WriteFile(composer, []byte("echo mine\n"), 0o644)
	if w := Sync(home); len(w) != 0 {
		t.Fatalf("warnings %q", w)
	}
	if b, _ := os.ReadFile(php); !strings.Contains(string(b), `exec php "$@"`) {
		t.Errorf("marker file not rewritten: %q", b)
	}
	if b, _ := os.ReadFile(composer); string(b) != "echo mine\n" {
		t.Errorf("user file touched: %q", b)
	}
}

func TestSyncUnwritable(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "commands", Web)
	os.MkdirAll(dir, 0o755)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if f := filepath.Join(dir, ".probe"); os.WriteFile(f, nil, 0o644) == nil {
		os.Remove(f)
		t.Skip("directory permissions are not enforced (root)")
	}
	if w := Sync(home); len(w) == 0 {
		t.Error("no warning for an unwritable folder")
	}
}
