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
	mustContain(t, out, "KEY", "php", "8.3", "shop.test", "varnish", "varnish_version")
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

func newGlobalHome(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	stubDocker(t, "")
	if body != "" {
		os.WriteFile(config.GlobalPath(), []byte(body), 0o644)
	}
	return home
}

func readGlobal(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(config.GlobalPath())
	return string(b)
}

func TestConfigGlobalCreatesFile(t *testing.T) {
	newGlobalHome(t, "")
	out, _, err := runConfig(t, newConfigGlobalCmd, "--mysql-version=8.4")
	if err != nil {
		t.Fatal(err)
	}
	if got := readGlobal(t); got != "versions:\n  mysql: \"8.4\"\n" {
		t.Fatalf("%q", got)
	}
	mustContain(t, out, "versions.mysql: 8.0 → 8.4", "Run: lodev services up to apply")
}

func TestConfigGlobalCreatesHome(t *testing.T) {
	t.Setenv("LODEV_HOME", filepath.Join(t.TempDir(), "nested", "home"))
	stubDocker(t, "")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--http-port=8081"); err != nil {
		t.Fatal(err)
	}
	if got := readGlobal(t); got != "ports:\n  http: 8081\n" {
		t.Fatalf("%q", got)
	}
}

func TestConfigGlobalEnableDisable(t *testing.T) {
	newGlobalHome(t, "")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--enable-services=rabbitmq,mariadb", "--disable-services=opensearch"); err != nil {
		t.Fatal(err)
	}
	g, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	if !g.Services["rabbitmq"] || !g.Services["mariadb"] || g.Services["opensearch"] {
		t.Fatalf("%v", g.Services)
	}
}

func TestConfigGlobalFlagErrors(t *testing.T) {
	newGlobalHome(t, "")
	cases := [][2]string{
		{"--enable-services=redis --disable-services=redis", "--enable-services and --disable-services both name redis"},
		{"--enable-services=nope", `unknown service "nope" (allowed: mysql, redis,`},
		{"--http-port=abc", `--http-port "abc" is not a number`},
	}
	for _, c := range cases {
		_, _, err := runConfig(t, newConfigGlobalCmd, strings.Fields(c[0])...)
		if err == nil || !strings.HasPrefix(err.Error(), c[1]) {
			t.Errorf("%s: %v", c[0], err)
		}
	}
	if _, err := os.Stat(config.GlobalPath()); !os.IsNotExist(err) {
		t.Fatal("file created")
	}
}

func TestConfigGlobalInvalidKeepsFile(t *testing.T) {
	body := "# keep\nports:\n  dashboard: 8080\n"
	newGlobalHome(t, body)
	_, _, err := runConfig(t, newConfigGlobalCmd, "--https-port=8080")
	if err == nil || err.Error() != "not saved: ports.https and ports.dashboard are both 8080" {
		t.Fatal(err)
	}
	if readGlobal(t) != body {
		t.Fatal("file changed")
	}
}

func TestConfigGlobalDeleteKey(t *testing.T) {
	newGlobalHome(t, "")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--mysql-port=3306"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runConfig(t, newConfigGlobalCmd, "--mysql-port=")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readGlobal(t), "mysql:") {
		t.Fatalf("kept:\n%s", readGlobal(t))
	}
	mustContain(t, out, "ports.mysql: 3306 → (unset)")
}

func TestConfigGlobalDisableWarns(t *testing.T) {
	newGlobalHome(t, "")
	dir := t.TempDir()
	p, err := newProjectConfig(dir, "a", "laravel", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.SaveProject(p); err != nil {
		t.Fatal(err)
	}
	if err := (registry.Registry{"a": {Path: dir}}).Save(); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := runConfig(t, newConfigGlobalCmd, "--disable-services=redis")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, errOut, "project a uses redis")
}

func TestConfigGlobalShowTable(t *testing.T) {
	newGlobalHome(t, "")
	out, _, err := runConfig(t, newConfigGlobalCmd)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "ports.http", "80", "versions.mysql", "8.0")
}

func TestConfigGlobalUpdateCheck(t *testing.T) {
	newGlobalHome(t, "")
	out, _, err := runConfig(t, newConfigGlobalCmd, "--update-check=false")
	if err != nil {
		t.Fatal(err)
	}
	if got := readGlobal(t); got != "update_check: false\n" {
		t.Errorf("file: %q", got)
	}
	mustContain(t, out, "update_check: true → false")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--update-check="); err != nil {
		t.Fatal(err)
	}
	if got := readGlobal(t); strings.Contains(got, "update_check") {
		t.Errorf("--update-check= should delete the key: %q", got)
	}
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--update-check=maybe"); err == nil || !strings.Contains(err.Error(), `--update-check "maybe" is not true or false`) {
		t.Errorf("err = %v", err)
	}
	out, _, _ = runConfig(t, newConfigGlobalCmd)
	mustContain(t, out, "update_check", "true")
}

func TestConfigGlobalMongoDB(t *testing.T) {
	newGlobalHome(t, "")
	out, _, err := runConfig(t, newConfigGlobalCmd, "--mongodb-port=27017")
	if err != nil {
		t.Fatal(err)
	}
	if got := readGlobal(t); got != "ports:\n  mongodb: \"27017\"\n" {
		t.Errorf("file: %q", got)
	}
	mustContain(t, out, "ports.mongodb: (unset) → 27017")
	out, _, err = runConfig(t, newConfigGlobalCmd, "--mongodb-version=7")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "versions.mongodb: 8 → 7")
}

func TestConfigGlobalDockerTools(t *testing.T) {
	newGlobalHome(t, "")
	out, _, err := runConfig(t, newConfigGlobalCmd, "--use-docker-compose-system=true")
	if err != nil {
		t.Fatal(err)
	}
	if got := readGlobal(t); got != "use_docker_compose_system: true\n" {
		t.Errorf("file: %q", got)
	}
	mustContain(t, out, "use_docker_compose_system: false → true")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--use-docker-buildx-system=maybe"); err == nil || !strings.Contains(err.Error(), `--use-docker-buildx-system "maybe" is not true or false`) {
		t.Errorf("err = %v", err)
	}
	out, _, _ = runConfig(t, newConfigGlobalCmd)
	mustContain(t, out, "use_docker_compose_system", "use_docker_buildx_system")
}

func TestConfigVarnish(t *testing.T) {
	root := newShop(t)
	if _, _, err := runConfig(t, newConfigCmd, "--varnish=true", "--varnish-version=9.1"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, readShop(t, root), "varnish: true", `varnish_version: "9.1"`)
	before := readShop(t, root)
	if _, _, err := runConfig(t, newConfigCmd, "--webserver=none"); err == nil || readShop(t, root) != before {
		t.Error("varnish with webserver none was saved")
	}
	if _, _, err := runConfig(t, newConfigCmd, "--varnish=maybe"); err == nil || err.Error() != `--varnish "maybe" is not true or false` {
		t.Errorf("err = %v", err)
	}
	if _, _, err := runConfig(t, newConfigCmd, "--varnish=false", "--varnish-version="); err != nil {
		t.Fatal(err)
	}
	if got := readShop(t, root); strings.Contains(got, "varnish") {
		t.Errorf("varnish keys left:\n%s", got)
	}
}

func TestConfigGlobalDataDir(t *testing.T) {
	newGlobalHome(t, "")
	_, errOut, err := runConfig(t, newConfigGlobalCmd, "--data-dir=/srv/d", "--mount-volume=true", "--mysql-mount-path=/srv/m")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, readGlobal(t), "data_dir: /srv/d", "mount_volume: true", "mount_paths:", "mysql: /srv/m")
	mustContain(t, errOut, "data_dir changed: existing data stays in "+filepath.Join(config.Home(), "data"))
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--mount-volume=nah"); err == nil || err.Error() != `--mount-volume "nah" is not true or false` {
		t.Errorf("err = %v", err)
	}
	out, _, err := runConfig(t, newConfigGlobalCmd)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, "data_dir", "/srv/d", "mount_volume", "mount_paths.mysql")
	if _, _, err := runConfig(t, newConfigGlobalCmd, "--mysql-mount-path="); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readGlobal(t), "mysql: /srv/m") {
		t.Errorf("mount path kept:\n%s", readGlobal(t))
	}
}

func TestConfigGlobalMountPathMoveWarns(t *testing.T) {
	newGlobalHome(t, "mount_volume: true\n")
	old := filepath.Join(config.Home(), "data", "mysql")
	_, errOut, err := runConfig(t, newConfigGlobalCmd, "--mysql-mount-path=/srv/m")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, errOut, "mysql data folder changed: move "+old+" to /srv/m before `lodev services up`, or mysql starts from its old Docker volume")
	// Turning mount_volume on for the first time is the wanted copy, not a move.
	newGlobalHome(t, "")
	if _, errOut, _ = runConfig(t, newConfigGlobalCmd, "--mount-volume=true"); strings.Contains(errOut, "data folder changed") {
		t.Errorf("warned on first enable: %s", errOut)
	}
}
