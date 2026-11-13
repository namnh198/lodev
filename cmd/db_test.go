package cmd

import (
	"bytes"
	"compress/gzip"
	"fmt"
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
		tg, err := dbTargetFor(c.args, c.service, "")
		if err != nil || tg.service+" "+tg.db != c.want {
			t.Errorf("%v %q: %+v %v", c.args, c.service, tg, err)
		}
	}
	if _, err := dbTargetFor(nil, "postgres", ""); err == nil || !strings.Contains(err.Error(), "mysql, mariadb, mongodb") {
		t.Errorf("postgres: %v", err)
	}
	t.Chdir(t.TempDir()) // outside a project
	if tg, err := dbTargetFor([]string{"x"}, "", ""); err != nil || tg.service != "mysql" {
		t.Errorf("outside: %+v %v", tg, err)
	}
	if _, err := dbTargetFor(nil, "", ""); err == nil || err.Error() != "give the database name (not in a lodev project)" {
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
		if got := strings.Join(dbDumpArgs(dbTarget{c.svc, "shop"}, nil), " "); got != c.dump {
			t.Errorf("%s dump:\n got %s\nwant %s", c.svc, got, c.dump)
		}
		if got := strings.Join(dbRestoreArgs(dbTarget{c.svc, "shop"}, nil), " "); got != c.restore {
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

// TestDBTargetRejectsBadNames: a name that would break the backtick-quoted SQL fails before Docker.
func TestDBTargetRejectsBadNames(t *testing.T) {
	newShop(t)
	for _, name := range []string{"a`b", "a;b", "a b", "", "-A", "--force", "a$b"} {
		if _, err := dbTargetFor([]string{name}, "mysql", ""); err == nil || err.Error() != fmt.Sprintf("database name %q: use A-Z a-z 0-9 _ - (not first)", name) {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"shop_2", "my-db", "_x"} {
		if _, err := dbTargetFor([]string{name}, "mysql", ""); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
}

func TestDBTargetDBNameFlag(t *testing.T) {
	newShop(t)
	if tg, err := dbTargetFor(nil, "", "other"); err != nil || tg.service+" "+tg.db != "mysql other" {
		t.Errorf("-n other: %+v %v", tg, err)
	}
	if _, err := dbTargetFor([]string{"other"}, "", "other"); err != nil {
		t.Errorf("same twice: %v", err)
	}
	if _, err := dbTargetFor([]string{"a"}, "", "b"); err == nil || err.Error() != `the database is "a" as an argument and "b" with --db-name; give one` {
		t.Errorf("both: %v", err)
	}
	if _, err := dbTargetFor(nil, "", "-A"); err == nil || !strings.Contains(err.Error(), "database name") {
		t.Errorf("-n -A: %v", err)
	}
}

func TestDBExportDBNameFlag(t *testing.T) {
	newShop(t)
	calls, _ := stubDBStream(t, "")
	if _, _, err := runConfig(t, newDBExportCmd, "-n", "other", "-f", "-"); err != nil {
		t.Fatal(err)
	}
	if got := (*calls)[len(*calls)-1]; got[len(got)-1] != "other" {
		t.Errorf("dump %q", got)
	}
}

func TestSplitArgs(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"--skip-add-locks", []string{"--skip-add-locks"}},
		{`--no-data --where="id > 10"`, []string{"--no-data", "--where=id > 10"}},
		{`'a b' c`, []string{"a b", "c"}},
		{`a\ b`, []string{"a b"}},
		{`x ""`, []string{"x", ""}},
		{`"say \"hi\" \\ ok"`, []string{`say "hi" \ ok`}},
		{"; rm -rf /", []string{";", "rm", "-rf", "/"}},
		{"$(id) `id`", []string{"$(id)", "`id`"}},
	} {
		got, err := splitArgs(c.in)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%q: %q %v", c.in, got, err)
		}
	}
	for in, want := range map[string]string{`'open`: "unclosed quote", `"open`: "unclosed quote", `a\`: "trailing backslash"} {
		if _, err := splitArgs(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", in, err)
		}
	}
}

func TestDBDumpRestoreExtra(t *testing.T) {
	extra := []string{"--x", "--y=1"}
	for _, c := range []struct{ svc, dump, restore string }{
		{"mysql", "--no-tablespaces --x --y=1 shop", "mysql -uroot --x --y=1 shop"},
		{"mariadb", "--no-tablespaces --x --y=1 shop", "mariadb -uroot --x --y=1 shop"},
		{"mongodb", "--archive --x --y=1", "--nsTo=shop.$col$ --x --y=1"},
	} {
		if got := strings.Join(dbDumpArgs(dbTarget{c.svc, "shop"}, extra), " "); !strings.HasSuffix(got, c.dump) {
			t.Errorf("%s dump: %s", c.svc, got)
		}
		if got := strings.Join(dbRestoreArgs(dbTarget{c.svc, "shop"}, extra), " "); !strings.HasSuffix(got, c.restore) {
			t.Errorf("%s restore: %s", c.svc, got)
		}
	}
}

func TestDBExportExtraCommand(t *testing.T) {
	newShop(t)
	calls, _ := stubDBStream(t, "")
	if _, _, err := runConfig(t, newDBExportCmd, "-e", "--skip-add-locks", "-e", `--where="id > 10"`, "-f", "-"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join((*calls)[len(*calls)-1], " "); !strings.HasSuffix(got, "--no-tablespaces --skip-add-locks --where=id > 10 shop") {
		t.Errorf("dump %s", got)
	}
	out := filepath.Join(t.TempDir(), "out.sql")
	n := len(*calls)
	if _, _, err := runConfig(t, newDBExportCmd, "-e", `'open`, "-f", out); err == nil || !strings.Contains(err.Error(), "unclosed quote") {
		t.Errorf("bad quote: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) || len(*calls) != n {
		t.Errorf("bad quote still ran: file %v, calls %d→%d", err, n, len(*calls))
	}
}
