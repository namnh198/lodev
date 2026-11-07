package cmd

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// stubDBStream records docker exec streams: what the "container" got on stdin, and answers dump.
func stubDBStream(t *testing.T, dump string) (calls *[][]string, stdin *bytes.Buffer) {
	t.Helper()
	os, or, oo := dockerStream, containerRunning, dockerOutput
	t.Cleanup(func() { dockerStream, containerRunning, dockerOutput = os, or, oo })
	var c [][]string
	in := &bytes.Buffer{}
	dockerStream = func(args []string, r io.Reader, w io.Writer) error {
		c = append(c, args)
		if r != nil {
			io.Copy(in, r)
		}
		if w != nil {
			io.WriteString(w, dump)
		}
		return nil
	}
	containerRunning = func(string) bool { return true }
	dockerOutput = func(a ...string) (string, error) { c = append(c, a); return "", nil }
	return &c, in
}

func TestDBTarget(t *testing.T) {
	root := newShop(t) // type php, services [mysql], name shop
	_ = root
	for _, c := range []struct {
		args          []string
		service, want string
	}{
		{nil, "", "mysql shop"},
		{[]string{"other"}, "", "mysql other"},
		{nil, "mongodb", "mongodb shop"},
	} {
		tg, err := dbTargetFor(c.args, c.service)
		if err != nil || tg.service+" "+tg.db != c.want {
			t.Errorf("%v %q: %+v %v", c.args, c.service, tg, err)
		}
	}
	if _, err := dbTargetFor(nil, "postgres"); err == nil || !strings.Contains(err.Error(), "mysql, mariadb, mongodb") {
		t.Errorf("postgres: %v", err)
	}
	t.Chdir(t.TempDir()) // outside a project
	if tg, err := dbTargetFor([]string{"x"}, ""); err != nil || tg.service != "mysql" {
		t.Errorf("outside: %+v %v", tg, err)
	}
	if _, err := dbTargetFor(nil, ""); err == nil || err.Error() != "give the database name (not in a lodev project)" {
		t.Errorf("no name: %v", err)
	}
}

func TestDBCommands(t *testing.T) {
	for _, c := range []struct{ svc, dump, restore string }{
		{"mysql", "exec -i -e MYSQL_PWD=root lodev-mysql mysqldump -uroot --single-transaction --routines --triggers --events --no-tablespaces shop",
			"exec -i -e MYSQL_PWD=root lodev-mysql mysql -uroot shop"},
		{"mariadb", "exec -i -e MYSQL_PWD=root lodev-mariadb mariadb-dump -uroot --single-transaction --routines --triggers --events --no-tablespaces shop",
			"exec -i -e MYSQL_PWD=root lodev-mariadb mariadb -uroot shop"},
		{"mongodb", "exec -i lodev-mongodb mongodump --uri=mongodb://root:root@localhost:27017/?authSource=admin --db=shop --archive",
			"exec -i lodev-mongodb mongorestore --uri=mongodb://root:root@localhost:27017/?authSource=admin --archive --drop --nsFrom=$db$.$col$ --nsTo=shop.$col$"},
	} {
		if got := strings.Join(dbDumpArgs(dbTarget{c.svc, "shop"}), " "); got != c.dump {
			t.Errorf("%s dump:\n got %s\nwant %s", c.svc, got, c.dump)
		}
		if got := strings.Join(dbRestoreArgs(dbTarget{c.svc, "shop"}), " "); got != c.restore {
			t.Errorf("%s restore:\n got %s\nwant %s", c.svc, got, c.restore)
		}
	}
}

func TestDBDefaultFile(t *testing.T) {
	at := time.Date(2026, 11, 7, 14, 5, 9, 0, time.UTC)
	for _, c := range []struct {
		tg   dbTarget
		gz   bool
		want string
	}{
		{dbTarget{"mysql", "shop"}, false, "shop-20261107-140509.sql"},
		{dbTarget{"mariadb", "shop"}, true, "shop-20261107-140509.sql.gz"},
		{dbTarget{"mongodb", "shop"}, true, "shop-20261107-140509.archive.gz"},
	} {
		if got := dbDefaultFile(c.tg, c.gz, at); got != c.want {
			t.Errorf("%+v gz=%v: %s", c.tg, c.gz, got)
		}
	}
}

func TestDBExportImportGzipRoundTrip(t *testing.T) {
	newShop(t)
	_, stdin := stubDBStream(t, "-- dump\nCREATE TABLE t (id int);\n")
	f := filepath.Join(t.TempDir(), "d.sql.gz")
	if _, _, err := runConfig(t, newDBExportCmd, "-f", f, "-c", "gz"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(f)
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	plain, _ := io.ReadAll(zr)
	if string(plain) != "-- dump\nCREATE TABLE t (id int);\n" {
		t.Errorf("export: %q", plain)
	}
	// import detects gzip by its magic bytes, no -c needed
	if _, _, err := runConfig(t, newDBImportCmd, "-f", f); err != nil {
		t.Fatal(err)
	}
	if stdin.String() != "-- dump\nCREATE TABLE t (id int);\n" {
		t.Errorf("import stdin: %q", stdin.String())
	}
}

func TestDBImportPlainCreatesDatabase(t *testing.T) {
	newShop(t)
	calls, stdin := stubDBStream(t, "")
	f := filepath.Join(t.TempDir(), "d.sql")
	os.WriteFile(f, []byte("SELECT 1;\n"), 0o644)
	if _, _, err := runConfig(t, newDBImportCmd, "other", "-f", f); err != nil {
		t.Fatal(err)
	}
	if stdin.String() != "SELECT 1;\n" {
		t.Errorf("stdin %q", stdin.String())
	}
	if got := strings.Join((*calls)[0], " "); got != "exec lodev-mysql mysql -uroot -proot -e CREATE DATABASE IF NOT EXISTS `other`" {
		t.Errorf("first call %q", got)
	}
}

func TestDBExportDefaultFileAndStdout(t *testing.T) {
	root := newShop(t)
	stubDBStream(t, "x")
	out, _, err := runConfig(t, newDBExportCmd, "-f", "-")
	if err != nil || out != "x" {
		t.Fatalf("stdout export: %q %v", out, err)
	}
	if _, _, err := runConfig(t, newDBExportCmd); err != nil {
		t.Fatal(err)
	}
	m, _ := filepath.Glob(filepath.Join(root, "shop-*.sql"))
	if len(m) != 1 {
		t.Errorf("default file: %v", m)
	}
}

func TestDBErrors(t *testing.T) {
	newShop(t)
	stubDBStream(t, "")
	if _, _, err := runConfig(t, newDBExportCmd, "-c", "zip"); err == nil || err.Error() != `--compress "zip": only gz is supported` {
		t.Errorf("zip: %v", err)
	}
	if _, _, err := runConfig(t, newDBImportCmd); err == nil || err.Error() != "give the dump with -f <file> (or -f - for stdin)" {
		t.Errorf("no file: %v", err)
	}
	containerRunning = func(string) bool { return false }
	if _, _, err := runConfig(t, newDBExportCmd, "-s", "mariadb"); err == nil || err.Error() != "lodev-mariadb is not running (lodev service enable mariadb)" {
		t.Errorf("not running: %v", err)
	}
}

func TestDBCommandsNeedDocker(t *testing.T) {
	for _, c := range []string{"lodev db-export", "lodev db-import"} {
		if !needsDocker[c] {
			t.Errorf("%s not in needsDocker", c)
		}
	}
	_ = slices.Contains[[]string]
}
