package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlagKey(t *testing.T) {
	for in, want := range map[string]string{"--app-key": "APP_KEY", "--x": "X", "--MAGE_RUN_CODE": "MAGE_RUN_CODE"} {
		if got := flagKey(in); got != want {
			t.Errorf("%s → %s", in, got)
		}
	}
}

func readDotenv(t *testing.T, root string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(root, ".lodev", ".env"))
	return string(b)
}

func TestDotenvSet(t *testing.T) {
	root := newShop(t)
	out, _, err := runConfig(t, newDotenvCmd, "set", "--app-key=abc", "--mage-run-code=default")
	if err != nil {
		t.Fatal(err)
	}
	if got := readDotenv(t, root); got != "APP_KEY=abc\nMAGE_RUN_CODE=default\n" {
		t.Errorf("file: %q", got)
	}
	if fi, _ := os.Stat(filepath.Join(root, ".lodev", ".env")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if gi, _ := os.ReadFile(filepath.Join(root, ".lodev", ".gitignore")); !strings.Contains(string(gi), "\n/.env\n") {
		t.Errorf(".lodev/.gitignore does not ignore .env: %q", gi)
	}
	if out != "Set APP_KEY, MAGE_RUN_CODE in .lodev/.env; run lodev restart to apply\n" {
		t.Errorf("out: %q", out)
	}
	writeDotenv(t, root, "# mine\nAPP_KEY=abc\n")
	if _, _, err := runConfig(t, newDotenvCmd, "set", "--app-key=x"); err != nil {
		t.Fatal(err)
	}
	if got := readDotenv(t, root); got != "# mine\nAPP_KEY=x\n" {
		t.Errorf("comment lost: %q", got)
	}
}

func TestDotenvSetErrors(t *testing.T) {
	root := newShop(t)
	writeDotenv(t, root, "A=1\n")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"set", "--app-key"}, "--app-key needs a value (--app-key=value)"},
		{[]string{"set", "--ok=1", "--lodev-uid=1"}, "environment: LODEV_UID is reserved for lodev"},
		{[]string{"set"}, "usage: lodev dotenv set --key=value [...]"},
	} {
		if _, _, err := runConfig(t, newDotenvCmd, c.args...); err == nil || err.Error() != c.want {
			t.Errorf("%v: err = %v", c.args, err)
		}
		if got := readDotenv(t, root); got != "A=1\n" {
			t.Errorf("%v changed the file: %q", c.args, got)
		}
	}
}

func TestDotenvGet(t *testing.T) {
	root := newShop(t)
	writeDotenv(t, root, "APP_KEY=abc\n")
	if out, _, err := runConfig(t, newDotenvCmd, "get", "--app-key"); err != nil || out != "abc\n" {
		t.Errorf("%q, %v", out, err)
	}
	if _, _, err := runConfig(t, newDotenvCmd, "get", "--nope"); err == nil || err.Error() != "NOPE is not set in .lodev/.env" {
		t.Errorf("missing: %v", err)
	}
	for _, args := range [][]string{{"get"}, {"get", "--a", "--b"}} {
		if _, _, err := runConfig(t, newDotenvCmd, args...); err == nil || err.Error() != "usage: lodev dotenv get --key" {
			t.Errorf("%v: %v", args, err)
		}
	}
}
