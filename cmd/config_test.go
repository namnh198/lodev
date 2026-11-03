package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
)

const shopConf = "# mine\nname: shop\ntype: php\nphp: \"8.3\"\nservices: [mysql]\n"

// newShop makes a project "shop" in a temp folder, cds into it and stubs Docker.
func newShop(t *testing.T) string {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
	writeShop(t, root, shopConf)
	t.Chdir(root)
	stubDocker(t, "")
	return root
}

func writeShop(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(config.ProjectPath(root), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readShop(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(config.ProjectPath(root))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func runConfig(t *testing.T, mk func() *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	_, e, restore := ui.Capture(false)
	defer restore()
	c := mk()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs(args)
	err = c.Execute()
	return out.String(), e.String(), err
}

func mustContain(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestConfigExample(t *testing.T) {
	root := newShop(t)
	out, _, err := runConfig(t, newConfigCmd, "--php-version=8.4", "--nodejs-version=24")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, readShop(t, root), "# mine", `php: "8.4"`, `nodejs: "24"`)
	mustContain(t, out, "php: 8.3 → 8.4", "nodejs: (unset) → 24", "Applies on the next lodev start")
}

func TestConfigRunningHint(t *testing.T) {
	newShop(t)
	stubDocker(t, "running")
	out, _, err := runConfig(t, newConfigCmd, "--php-version=8.4")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "Run: lodev restart to apply")
}

func TestConfigInvalidKeepsFile(t *testing.T) {
	root := newShop(t)
	for _, arg := range []string{"--php-version=9.1", "--services=mysql,mariadb", "--domains=a.com", "--docroot=../x"} {
		_, _, err := runConfig(t, newConfigCmd, arg)
		if err == nil || !strings.HasPrefix(err.Error(), "not saved: ") {
			t.Errorf("%s: %v", arg, err)
		}
		if got := readShop(t, root); got != shopConf {
			t.Errorf("%s changed the file:\n%s", arg, got)
		}
	}
}

func TestConfigFlagErrors(t *testing.T) {
	root := newShop(t)
	for flag, want := range map[string]string{"--type=": "--type cannot be empty", "--php-version=": "--php-version cannot be empty"} {
		if _, _, err := runConfig(t, newConfigCmd, flag); err == nil || err.Error() != want {
			t.Errorf("%s: %v", flag, err)
		}
	}
	if readShop(t, root) != shopConf {
		t.Fatal("file changed")
	}
}

func TestConfigNotAProject(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	_, _, err := runConfig(t, newConfigCmd, "--php-version=8.4")
	if err == nil || err.Error() != "no lodev project here (.lodev/config.yaml not found). Create it first: lodev create" {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lodev")); !os.IsNotExist(err) {
		t.Fatal(".lodev created")
	}
}

func TestConfigFromSubfolder(t *testing.T) {
	root := newShop(t)
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	t.Chdir(filepath.Join(root, "sub"))
	if _, _, err := runConfig(t, newConfigCmd, "--php-version=8.4"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readShop(t, root), `php: "8.4"`)
}

func TestConfigDomainConflict(t *testing.T) {
	newShop(t)
	other, err := newProjectConfig(t.TempDir(), "other", "php", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SaveProject(other); err != nil {
		t.Fatal(err)
	}
	if err := (registry.Registry{"other": {Path: other.Root, Domains: []string{"x.test"}}}).Save(); err != nil {
		t.Fatal(err)
	}
	_, _, err = runConfig(t, newConfigCmd, "--add-domains=x.test")
	if err == nil || !strings.HasPrefix(err.Error(), `not saved: domain x.test is already used by project "other"`) {
		t.Fatal(err)
	}
}

func TestConfigLists(t *testing.T) {
	root := newShop(t)
	_, _, err := runConfig(t, newConfigCmd, "--services=mysql", "--add-services=redis", "--remove-services=mysql", "--php-extensions=imagick")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, readShop(t, root), "services: [redis]", "php_extensions: [imagick]")
}

func TestConfigRemoveAllServices(t *testing.T) {
	root := newShop(t)
	if _, _, err := runConfig(t, newConfigCmd, "--remove-services=mysql"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readShop(t, root), "services: []")
}

func TestConfigAddDomainKeepsDefault(t *testing.T) {
	root := newShop(t)
	if _, _, err := runConfig(t, newConfigCmd, "--add-domains=x.test"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readShop(t, root), "domains: [shop.test, x.test]")
}

func TestConfigDeleteKey(t *testing.T) {
	root := newShop(t)
	if _, _, err := runConfig(t, newConfigCmd, "--nodejs-version=24"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runConfig(t, newConfigCmd, "--nodejs-version=")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readShop(t, root), "nodejs") {
		t.Fatalf("nodejs kept:\n%s", readShop(t, root))
	}
	mustContain(t, out, "nodejs: 24 → (unset)")
}

func TestConfigDocrootDot(t *testing.T) {
	root := newShop(t)
	os.MkdirAll(filepath.Join(root, "public"), 0o755)
	if _, _, err := runConfig(t, newConfigCmd, "--docroot=public"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runConfig(t, newConfigCmd, "--docroot=."); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readShop(t, root), `docroot: ""`)
}

func TestConfigNoChangeKeepsFile(t *testing.T) {
	root := newShop(t)
	body := shopConf + "domains:\n    - shop.test\n"
	writeShop(t, root, body)
	out, _, err := runConfig(t, newConfigCmd, "--php-version=8.3")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "nothing changed")
	if readShop(t, root) != body {
		t.Fatalf("rewritten:\n%s", readShop(t, root))
	}
}

func TestConfigFixesInvalidFile(t *testing.T) {
	root := newShop(t)
	writeShop(t, root, strings.Replace(shopConf, `"8.3"`, `"9.9"`, 1))
	if _, _, err := runConfig(t, newConfigCmd, "--php-version=8.4"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadProject(root); err != nil {
		t.Fatal(err)
	}
}

func TestConfigShowTable(t *testing.T) {
	root := newShop(t)
	out, _, err := runConfig(t, newConfigCmd)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "KEY", "php", "8.3", "shop.test")
	if readShop(t, root) != shopConf {
		t.Fatal("file changed")
	}
}

func TestConfigWarnings(t *testing.T) {
	root := newShop(t)
	_, errOut, err := runConfig(t, newConfigCmd, "--docroot=web", "--user=John.Doe")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, errOut, "docroot web does not exist yet", `user "John.Doe" will be john_doe in the container`)
	mustContain(t, readShop(t, root), "docroot: web", "user: John.Doe")
}
