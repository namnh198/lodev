package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
)

func TestMagentoSQL(t *testing.T) {
	want := []string{
		"UPDATE `shop`.`core_config_data` SET value='https://shop.test/' WHERE path IN ('web/unsecure/base_url','web/secure/base_url','web/unsecure/base_link_url','web/secure/base_link_url')",
		"UPDATE `shop`.`core_config_data` SET value=NULL WHERE path IN ('web/unsecure/base_static_url','web/secure/base_static_url','web/unsecure/base_media_url','web/secure/base_media_url')",
		"DELETE FROM `shop`.`core_config_data` WHERE path='web/cookie/cookie_domain'",
	}
	if got := magentoSQL("shop", "", "https://shop.test/"); !slices.Equal(got, want) {
		t.Errorf("got\n%s", strings.Join(got, "\n"))
	}
	if got := magentoSQL("shop", "m2_", "https://shop.test/"); !strings.Contains(got[0], "`shop`.`m2_core_config_data`") {
		t.Errorf("prefix: %s", got[0])
	}
}

// stubMagento answers the prefix read with prefix and records SQL and web-container commands.
func stubMagento(t *testing.T, webUp bool, prefix string) (sql, web *[]string) {
	t.Helper()
	oldOut, oldRun, oldUp, oldReady := dockerOutput, dockerRun, containerRunning, userReady
	t.Cleanup(func() { dockerOutput, dockerRun, containerRunning, userReady = oldOut, oldRun, oldUp, oldReady })
	userReady = func(*config.Project, string) bool { return true }
	containerRunning = func(c string) bool { return webUp || !strings.HasSuffix(c, "-web") }
	var s, w []string
	dockerOutput = func(a ...string) (string, error) {
		if slices.Contains(a, "-r") {
			w = append(w, a[len(a)-1])
			return prefix, nil
		}
		if i := slices.Index(a, "-e"); i >= 0 && strings.HasPrefix(a[len(a)-1], "UPDATE") || strings.HasPrefix(a[len(a)-1], "DELETE") {
			s = append(s, a[len(a)-1])
		}
		return "", nil
	}
	dockerRun = func(a ...string) error { w = append(w, strings.Join(a[len(a)-3:], " ")); return nil }
	return &s, &w
}

func magentoProject() *config.Project {
	return &config.Project{Name: "shop", Type: "magento", Webserver: "nginx", Services: []string{"mysql"}, Domains: []string{"shop.test"}}
}

func TestMagentoAfterImport(t *testing.T) {
	sql, web := stubMagento(t, true, "m2_")
	if err := magentoAfterImport(magentoProject(), dbTarget{"mysql", "shop"}); err != nil {
		t.Fatal(err)
	}
	if len(*sql) != 3 || !strings.Contains((*sql)[0], "`m2_core_config_data`") {
		t.Errorf("sql %q", *sql)
	}
	wantPHP := `$e = include "app/etc/env.php"; echo $e["db"]["table_prefix"] ?? "";`
	if len(*web) != 2 || (*web)[0] != wantPHP || (*web)[1] != "php bin/magento cache:flush" {
		t.Errorf("web %q", *web)
	}
	sql, web = stubMagento(t, false, "ignored")
	if err := magentoAfterImport(magentoProject(), dbTarget{"mysql", "shop"}); err != nil || len(*sql) != 3 || len(*web) != 0 {
		t.Errorf("web down: sql %q web %q %v", *sql, *web, err)
	}
	if !strings.Contains((*sql)[0], "`shop`.`core_config_data`") {
		t.Errorf("web down must use no prefix: %s", (*sql)[0])
	}
}

func TestMagentoPrefixRejected(t *testing.T) {
	sql, _ := stubMagento(t, true, "a;DROP")
	err := magentoAfterImport(magentoProject(), dbTarget{"mysql", "shop"})
	if err == nil || err.Error() != `Magento: table prefix "a;DROP" is not valid` || len(*sql) != 0 {
		t.Errorf("%v, sql %q", err, *sql)
	}
}

func TestDBImportMagento(t *testing.T) {
	root := newShop(t)
	f := filepath.Join(t.TempDir(), "d.sql")
	os.WriteFile(f, []byte("SELECT 1;\n"), 0o644)
	run := func(conf string, args ...string) []string {
		writeShop(t, root, conf)
		stubDBStream(t, "")
		sql, web := stubMagento(t, true, "")
		if _, _, err := runConfig(t, newDBImportCmd, append([]string{"-f", f}, args...)...); err != nil {
			t.Fatal(err)
		}
		return append(*sql, *web...)
	}
	magento := strings.Replace(shopConf, "type: php", "type: magento", 1) + "domains: [shop.test]\nhooks:\n  post-db-import: [\"echo hook\"]\n"
	got := run(magento)
	if len(got) < 4 || !strings.HasPrefix(got[0], "UPDATE") || !strings.Contains(strings.Join(got, "|"), "cache:flush") ||
		!strings.HasSuffix(strings.Join(got, "|"), "echo hook") {
		t.Errorf("magento, then hook: %q", got)
	}
	for _, c := range []struct {
		conf string
		args []string
	}{{shopConf, nil}, {magento, []string{"other"}}, {magento, []string{"--no-magento"}}} {
		if got := run(c.conf, c.args...); slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "UPDATE") }) {
			t.Errorf("%v: Magento step ran: %q", c.args, got)
		}
	}
}

// lodev's PHP shows errors on stdout; a missing env.php must not end up as the "prefix".
func TestMagentoPrefixReadSendsErrorsToStderr(t *testing.T) {
	oldOut, oldUp, oldRun, oldReady := dockerOutput, containerRunning, dockerRun, userReady
	t.Cleanup(func() { dockerOutput, containerRunning, dockerRun, userReady = oldOut, oldUp, oldRun, oldReady })
	userReady = func(*config.Project, string) bool { return true }
	containerRunning = func(string) bool { return true }
	dockerRun = func(...string) error { return nil }
	var phpArgs []string
	dockerOutput = func(a ...string) (string, error) {
		if slices.Contains(a, "-r") {
			phpArgs = a
		}
		return "", nil
	}
	if err := magentoAfterImport(magentoProject(), dbTarget{"mysql", "shop"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(phpArgs, "display_errors=stderr") {
		t.Errorf("php args %q", phpArgs)
	}
}
