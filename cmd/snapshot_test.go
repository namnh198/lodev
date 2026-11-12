package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"lodev/internal/config"
)

func TestSnapshotTargets(t *testing.T) {
	p := &config.Project{Name: "shop", Services: []string{"mysql"}}
	if got := snapshotTargets(p); !slices.Equal(got, []dbTarget{{"mysql", "shop"}}) {
		t.Errorf("mysql: %v", got)
	}
	p.Services = []string{"mariadb", "mongodb"}
	if got := snapshotTargets(p); !slices.Equal(got, []dbTarget{{"mariadb", "shop"}, {"mongodb", "shop"}}) {
		t.Errorf("mariadb+mongodb: %v", got)
	}
}

// snapshotShop is the shop project with services, docker stubbed: dumps write "DUMP <svc>",
// restores record what they read; failOn fails the call whose args contain it.
func snapshotShop(t *testing.T, services, failOn string) (root string, restored *[]string) {
	t.Helper()
	root = newShop(t)
	writeShop(t, root, strings.Replace(shopConf, "services: [mysql]", "services: ["+services+"]", 1))
	stubDBStream(t, "")
	var got []string
	dockerStream = func(args []string, r io.Reader, w io.Writer) error {
		if failOn != "" && slices.Contains(args, failOn) {
			return errors.New("boom")
		}
		if w != nil {
			io.WriteString(w, "DUMP "+args[len(args)-1])
		}
		if r != nil {
			b, _ := io.ReadAll(r)
			got = append(got, string(b))
		}
		return nil
	}
	return root, &got
}

func TestSnapshotCreate(t *testing.T) {
	snapshotShop(t, "mysql", "")
	if _, _, err := runConfig(t, newSnapshotCmd, "create", "before"); err != nil {
		t.Fatal(err)
	}
	dir := snapshotDir("shop", "before")
	if _, err := os.Stat(filepath.Join(dir, "mysql.sql.gz")); err != nil {
		t.Errorf("dump: %v", err)
	}
	var m snapshotMeta
	b, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err := json.Unmarshal(b, &m); err != nil || !slices.Equal(m.Services, []string{"mysql"}) || m.Created.IsZero() {
		t.Errorf("meta %s %v", b, err)
	}
	if _, _, err := runConfig(t, newSnapshotCmd, "create", "before"); err == nil || err.Error() != "snapshot before exists (--force replaces it)" {
		t.Errorf("exists: %v", err)
	}
	if _, _, err := runConfig(t, newSnapshotCmd, "create", "before", "--force"); err != nil {
		t.Errorf("--force: %v", err)
	}
	if _, _, err := runConfig(t, newSnapshotCmd, "create", "a/b"); err == nil || err.Error() != `snapshot name "a/b": use A-Z a-z 0-9 . _ -` {
		t.Errorf("bad name: %v", err)
	}
	if _, _, err := runConfig(t, newSnapshotCmd, "create"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 2 || !regexp.MustCompile(`^\d{8}-\d{6}$`).MatchString(entries[0].Name()) {
		t.Errorf("default name: %v", entries)
	}
}

func TestSnapshotNoDatabase(t *testing.T) {
	root := newShop(t)
	writeShop(t, root, strings.Replace(shopConf, "services: [mysql]", "services: [redis]", 1))
	if _, _, err := runConfig(t, newSnapshotCmd, "create"); err == nil || err.Error() != "project shop has no database to snapshot" {
		t.Errorf("err = %v", err)
	}
}

func writeSnapshot(t *testing.T, name string, created time.Time, services ...string) {
	t.Helper()
	dir := snapshotDir("shop", name)
	os.MkdirAll(dir, 0o755)
	for _, s := range services {
		os.WriteFile(filepath.Join(dir, snapshotFile(s)), []byte("dump of "+s), 0o644)
	}
	b, _ := json.Marshal(snapshotMeta{Created: created, Services: services, Lodev: "dev"})
	os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o644)
}

func TestSnapshotList(t *testing.T) {
	snapshotShop(t, "mysql", "")
	now := time.Now()
	writeSnapshot(t, "old", now.Add(-time.Hour), "mysql")
	writeSnapshot(t, "new", now, "mysql", "mongodb")
	out, _, err := runConfig(t, newSnapshotCmd, "list")
	if err != nil || !strings.Contains(out, "NAME") || !strings.Contains(out, "DATABASES") || strings.Index(out, "new") > strings.Index(out, "old") ||
		!strings.Contains(out, "mysql, mongodb") {
		t.Errorf("list:\n%s %v", out, err)
	}
}

func TestSnapshotRestore(t *testing.T) {
	root, restored := snapshotShop(t, "mysql", "")
	writeShop(t, root, shopConf+"hooks:\n  post-snapshot-restore: [\"echo restored\"]\n")
	now := time.Now()
	writeSnapshot(t, "old", now.Add(-time.Hour), "mysql")
	writeSnapshot(t, "new", now, "mysql")
	ran := stubHookExec(t, true, nil)
	if _, _, err := runConfig(t, newSnapshotCmd, "restore"); err == nil || err.Error() != "aborted" || len(*restored) != 0 {
		t.Errorf("without --yes and no answer: %v %q", err, *restored)
	}
	if _, _, err := runConfig(t, newSnapshotCmd, "restore", "--yes"); err != nil || len(*restored) != 1 || (*restored)[0] != "dump of mysql" || len(*ran) != 1 {
		t.Errorf("newest: %q hooks %q %v", *restored, *ran, err)
	}
	if _, _, err := runConfig(t, newSnapshotCmd, "restore", "old", "--yes", "--no-hooks"); err != nil || len(*ran) != 1 {
		t.Errorf("--no-hooks: %q %v", *ran, err)
	}
}

func TestSnapshotRestorePartialFailure(t *testing.T) {
	root, _ := snapshotShop(t, "mysql, mongodb", "mongorestore")
	writeShop(t, root, strings.Replace(shopConf, "services: [mysql]", "services: [mysql, mongodb]", 1)+"hooks:\n  post-snapshot-restore: [\"echo restored\"]\n")
	writeSnapshot(t, "both", time.Now(), "mysql", "mongodb")
	ran := stubHookExec(t, true, nil)
	_, _, err := runConfig(t, newSnapshotCmd, "restore", "both", "--yes")
	if err == nil || !strings.Contains(err.Error(), "mongodb") || len(*ran) != 0 {
		t.Errorf("err %v, hooks %q", err, *ran)
	}
}

func TestSnapshotDelete(t *testing.T) {
	snapshotShop(t, "mysql", "")
	writeSnapshot(t, "x", time.Now(), "mysql")
	if _, _, err := runConfig(t, newSnapshotCmd, "delete", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snapshotDir("shop", "x")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("still there: %v", err)
	}
	if _, _, err := runConfig(t, newSnapshotCmd, "delete", "x"); err == nil || err.Error() != "no snapshot x for shop" {
		t.Errorf("unknown: %v", err)
	}
}
