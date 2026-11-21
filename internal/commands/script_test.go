package commands

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// runShipped runs the embedded host script name with bash, env added, and fakes (name → shell
// body) as executables first on PATH. Every fake also appends "<name> <args>" to $CALLS.
func runShipped(t *testing.T, name string, env map[string]string, fakes map[string]string, args ...string) (out string, calls []string, code int) {
	t.Helper()
	dir, callsFile := t.TempDir(), filepath.Join(t.TempDir(), "calls")
	for n, body := range fakes {
		script := "#!/bin/sh\necho \"" + n + " $*\" >> \"$CALLS\"\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var script string
	for _, c := range Shipped() {
		if c.Kind == Host && c.Name == name {
			script = c.Script
		}
	}
	if script == "" {
		t.Fatalf("no shipped host command %q", name)
	}
	x := exec.Command("bash", append([]string{"-c", script, name}, args...)...)
	x.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "CALLS="+callsFile)
	for k, v := range env {
		x.Env = append(x.Env, k+"="+v)
	}
	b, err := x.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(callsFile); err == nil {
		calls = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
	return string(b), calls, code
}

func TestOpenScript(t *testing.T) {
	env := map[string]string{"LODEV_BIN": "lodev"}
	fakes := func(osName, openBody string) map[string]string {
		return map[string]string{"lodev": "echo https://shop.test", "uname": "echo " + osName, "open": openBody, "xdg-open": openBody}
	}
	// without drops uname's own call, which every fake records.
	without := func(calls []string) []string {
		return slices.DeleteFunc(calls, func(c string) bool { return strings.HasPrefix(c, "uname ") })
	}
	for _, c := range []struct{ os, want string }{{"Darwin", "open https://shop.test"}, {"Linux", "xdg-open https://shop.test"}} {
		out, calls, code := runShipped(t, "open", env, fakes(c.os, "true"))
		if got := without(calls); code != 0 || out != "" || !slices.Equal(got, []string{"lodev url", c.want}) {
			t.Errorf("%s: %q %q %d", c.os, out, calls, code)
		}
	}

	out, calls, code := runShipped(t, "open", env, fakes("Darwin", "true"), "--print", "blog")
	if code != 0 || out != "https://shop.test\n" || !slices.Equal(calls, []string{"lodev url blog"}) {
		t.Errorf("--print: %q %q %d", out, calls, code)
	}

	out, calls, code = runShipped(t, "open", env, fakes("Darwin", "exit 1"), "blog")
	if code != 0 || !strings.Contains(out, "https://shop.test") || !strings.Contains(out, "could not open a browser") || !slices.Contains(calls, "open https://shop.test") {
		t.Errorf("failing open: %q %q %d", out, calls, code)
	}

	f := fakes("Darwin", "true")
	f["lodev"] = "exit 1"
	out, calls, code = runShipped(t, "open", env, f)
	if code != 1 || slices.Contains(calls, "open https://shop.test") {
		t.Errorf("failing lodev: %q %q %d", out, calls, code)
	}
}

func TestDBScripts(t *testing.T) {
	const url = "mysql://root:root@127.0.0.1:3306/shop"
	const dbeaver = "-con driver=mysql|host=127.0.0.1|port=3306|database=shop|user=root|password=root|name=lodev-shop|connect=true"
	env := func(kv ...string) map[string]string {
		e := map[string]string{"LODEV_PROJECT": "shop", "LODEV_DB_TYPE": "mysql", "LODEV_DB_HOST": "127.0.0.1",
			"LODEV_DB_PORT": "3306", "LODEV_DB_USER": "root", "LODEV_DB_PASSWORD": "root", "LODEV_DB_NAME": "shop"}
		for i := 0; i < len(kv); i += 2 {
			e[kv[i]] = kv[i+1]
		}
		return e
	}
	run := func(script, osName string, e map[string]string) (string, []string, int) {
		out, calls, code := runShipped(t, script, e, map[string]string{
			"uname": "echo " + osName, "open": "true", "xdg-open": "true", "dbeaver": "true", "mysql-workbench": "true"})
		return out, slices.DeleteFunc(calls, func(c string) bool { return strings.HasPrefix(c, "uname ") }), code
	}

	for _, c := range []struct{ script, os, want string }{
		{"tableplus", "Darwin", "open -a TablePlus " + url + "?name=lodev-shop"},
		{"tableplus", "Linux", "xdg-open " + url + "?name=lodev-shop"},
		{"sequelace", "Darwin", "open -a Sequel Ace " + url},
		{"dbeaver", "Darwin", "open -a DBeaver --args " + dbeaver},
		{"dbeaver", "Linux", "dbeaver " + dbeaver},
		{"mysqlworkbench", "Darwin", "open -a MySQLWorkbench --args --query root@127.0.0.1:3306"},
		{"mysqlworkbench", "Linux", "mysql-workbench --query root@127.0.0.1:3306"},
		{"tablepro", "Darwin", "open -a TablePro " + url + "?name=lodev-shop"},
	} {
		out, calls, code := run(c.script, c.os, env())
		if code != 0 || out != "" || !slices.Equal(calls, []string{c.want}) {
			t.Errorf("%s on %s: %q %q %d", c.script, c.os, out, calls, code)
		}
	}

	for _, s := range []string{"sequelace", "tablepro"} {
		out, calls, code := run(s, "Linux", env("LODEV_DB_PORT", "")) // the OS check comes first
		if code != 1 || !strings.Contains(out, "macOS only") || len(calls) != 0 {
			t.Errorf("%s on Linux: %q %q %d", s, out, calls, code)
		}
	}

	// PATH holds only a uname fake, so no real CLI can answer.
	bare := t.TempDir()
	if err := os.WriteFile(filepath.Join(bare, "uname"), []byte("#!/bin/sh\necho Linux\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ script, cli string }{{"tableplus", "xdg-open"}, {"dbeaver", "dbeaver"}, {"mysqlworkbench", "mysql-workbench"}} {
		out, _, code := runShipped(t, c.script, env("PATH", bare), nil)
		if code != 1 || !strings.Contains(out, c.cli+" is not installed") {
			t.Errorf("%s without %s: %q %d", c.script, c.cli, out, code)
		}
	}

	for _, s := range []string{"tableplus", "sequelace", "dbeaver", "mysqlworkbench", "tablepro"} {
		// MariaDB takes its own scheme or driver where the app has one.
		_, calls, code := run(s, "Darwin", env("LODEV_DB_TYPE", "mariadb"))
		wantMaria := s != "mysqlworkbench" && s != "sequelace" // neither has a MariaDB scheme or driver
		if len(calls) != 1 || code != 0 || strings.Contains(calls[0], "mariadb") != wantMaria {
			t.Errorf("%s mariadb: %q %d", s, calls, code)
		}
		for _, c := range []struct {
			name string
			env  map[string]string
			want string
		}{
			{"no port, mysql", env("LODEV_DB_PORT", ""), "MySQL is not reachable from this machine: lodev config-global --mysql-port=3306, then lodev service start"},
			{"no port, mariadb", env("LODEV_DB_TYPE", "mariadb", "LODEV_DB_PORT", ""), "MariaDB is not reachable from this machine: lodev config-global --mariadb-port=3307, then lodev service start"},
			{"no database", env("LODEV_DB_TYPE", ""), "has no MySQL/MariaDB service"},
			{"no project", env("LODEV_PROJECT", ""), "not in a lodev project"},
		} {
			out, calls, code := run(s, "Darwin", c.env)
			if code != 1 || !strings.Contains(out, c.want) || len(calls) != 0 {
				t.Errorf("%s, %s: %q %q %d", s, c.name, out, calls, code)
			}
		}
	}
}
