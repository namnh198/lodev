package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lodev/internal/config"
	"lodev/internal/hosts"
	"lodev/internal/registry"
	"lodev/internal/templates"
)

func TestExtraComposeFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".lodev")
	os.MkdirAll(dir, 0o755)
	for _, n := range []string{"b.compose.yaml", "a.compose.yml", "x.compose.yml.example", "lodev.compose-base.yml"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	got, err := extraComposeFiles(&config.Project{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.compose.yml"), filepath.Join(dir, "b.compose.yaml")}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestExtraComposeFilesBracketedPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "[x]")
	dir := filepath.Join(root, ".lodev")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "a.compose.yml"), nil, 0o644)
	got, err := extraComposeFiles(&config.Project{Root: root})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestComposeFileFallsBackToLegacy(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
	p := &config.Project{Name: "x", Root: root}
	if composeFile(p) != filepath.Join(root, ".lodev", "lodev.compose-full.yml") {
		t.Fatalf("neither file: %s", composeFile(p))
	}
	os.WriteFile(composeLegacy(p), nil, 0o644)
	if composeFile(p) != composeLegacy(p) {
		t.Fatalf("legacy only: %s", composeFile(p))
	}
	os.WriteFile(filepath.Join(root, ".lodev", "lodev.compose-full.yml"), nil, 0o644)
	if composeFile(p) == composeLegacy(p) {
		t.Fatal("full present must win")
	}
}

func TestMergeComposeFromLegacyCreatesFull(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
	p := &config.Project{Name: "x", Root: root}
	os.WriteFile(composeLegacy(p), []byte("legacy"), 0o644)
	defer func(o func(...string) (string, error)) { composeConfig = o }(composeConfig)
	composeConfig = func(...string) (string, error) { return "name: x", nil }
	if err := mergeCompose(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(composeFull(p)); string(b) != "name: x\n" {
		t.Fatalf("full = %q", b)
	}
	if b, _ := os.ReadFile(composeLegacy(p)); string(b) != "legacy" {
		t.Fatalf("legacy clobbered: %q", b)
	}
	if composeFile(p) != composeFull(p) {
		t.Fatalf("composeFile = %s", composeFile(p))
	}
}

func TestDropStaleProjectImage(t *testing.T) {
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)
	var got []string
	dockerOutput = func(a ...string) (string, error) { got = a; return "", nil }
	p := &config.Project{Name: "shop", PHP: "8.3"}
	if err := dropStaleProjectImage(p, "lodev/shop-web"); err != nil || got != nil {
		t.Fatalf("image in use must be kept: %v %v", got, err)
	}
	if err := dropStaleProjectImage(p, config.PHPImage("8.3")); err != nil || strings.Join(got, " ") != "rmi lodev/shop-web" {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestMergeCompose(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
	p := &config.Project{Name: "x", Root: root}
	full := composeFull(p)
	os.WriteFile(full, []byte("old"), 0o644)
	orig := composeConfig
	defer func() { composeConfig = orig }()

	composeConfig = func(...string) (string, error) { return "", errors.New("boom") }
	if err := mergeCompose(p); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(full); string(b) != "old" {
		t.Fatalf("full file clobbered: %q", b)
	}

	composeConfig = func(...string) (string, error) { return "name: x", nil }
	if err := mergeCompose(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(full); string(b) != "name: x\n" {
		t.Fatalf("got %q", b)
	}
}

func TestNewProjectConfigDetects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "My_Shop.v2")
	os.MkdirAll(filepath.Join(root, "bin"), 0o755)
	os.WriteFile(filepath.Join(root, "bin", "magento"), nil, 0o644)
	p, err := newProjectConfig(root, "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "my-shop-v2" || p.Type != "magento" || p.Docroot != "pub" || p.PHP != "8.3" ||
		p.Webserver != "nginx" || !slices.Equal(p.Services, []string{"mysql", "redis", "opensearch"}) || p.Root != root {
		t.Fatalf("%+v", p)
	}
}

func TestNewProjectConfigFlagsWin(t *testing.T) {
	p, err := newProjectConfig(t.TempDir(), "x", "laravel", "7.4", "none", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "x" || p.Type != "laravel" || p.PHP != "7.4" || p.Webserver != "none" || p.Docroot != "public" ||
		!slices.Equal(p.Services, []string{"mysql", "redis"}) {
		t.Fatalf("%+v", p)
	}
}

func TestNewProjectConfigRejectsInvalid(t *testing.T) {
	if _, err := newProjectConfig(t.TempDir(), "x", "php", "7.3", "", ""); err == nil {
		t.Fatal("php 7.3 accepted")
	}
}

func TestDeleteRefusesCopyOfAnotherProject(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	orig := t.TempDir()
	os.MkdirAll(filepath.Join(orig, ".lodev"), 0o755)
	os.WriteFile(filepath.Join(orig, ".lodev", "config.yaml"), nil, 0o644)
	reg := registry.Registry{"shop": {Path: orig, Domains: []string{"shop.test"}}}
	reg.Save()

	copyP, _ := newProjectConfig(t.TempDir(), "shop", "php", "", "", "")
	err := deleteProject(copyP, true, true)
	if err == nil || !strings.Contains(err.Error(), orig) {
		t.Fatalf("delete of a copy must refuse and name the original: %v", err)
	}
	if got, _ := registry.Load(); got["shop"].Path != orig {
		t.Fatal("original registry entry was removed")
	}
}

func TestDeleteRemovesProjectImage(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	hf := filepath.Join(t.TempDir(), "hosts")
	os.WriteFile(hf, []byte("127.0.0.1 localhost\n"), 0o644)
	defer func(o string) { hosts.File = o }(hosts.File)
	hosts.File = hf
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)

	p, _ := newProjectConfig(t.TempDir(), "shop", "php", "8.3", "", "")
	var got []string
	for _, rmiErr := range []error{nil, errors.New("Error response from daemon: No such image: lodev/shop-web:latest")} {
		got = nil
		dockerOutput = func(a ...string) (string, error) { got = a; return "", rmiErr }
		if err := deleteProject(p, false, true); err != nil {
			t.Fatalf("delete failed with rmi err %v: %v", rmiErr, err)
		}
		if strings.Join(got, " ") != "rmi lodev/shop-web" {
			t.Fatalf("docker call = %v", got)
		}
	}
	dockerOutput = func(...string) (string, error) { return "", errors.New("conflict: image in use") }
	if err := deleteProject(p, false, true); err == nil {
		t.Fatal("other rmi errors must surface")
	}
}

func confProject(t *testing.T, ws string) *config.Project {
	t.Helper()
	p, err := newProjectConfig(t.TempDir(), "app", "php", "8.3", ws, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func writeConf(t *testing.T, p *config.Project, body string) {
	t.Helper()
	os.MkdirAll(filepath.Join(p.Root, ".lodev"), 0o755)
	if err := os.WriteFile(WebserverConfFile(p), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteWebserverConfRegenerates(t *testing.T) {
	p := confProject(t, "nginx")
	writeConf(t, p, templates.GeneratedMarker+"\nold\n")
	if err := writeWebserverConf(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(WebserverConfFile(p)); strings.Contains(string(b), "old\n") || !strings.HasPrefix(string(b), templates.GeneratedMarker) {
		t.Fatalf("%q", b)
	}
}

func TestWriteWebserverConfUserManaged(t *testing.T) {
	for name, body := range map[string]string{
		"no marker":           "mine\n",
		"marker not 1st line": "# mine\n" + templates.GeneratedMarker + "\nx\n",
	} {
		p := confProject(t, "nginx")
		writeConf(t, p, body)
		if err := writeWebserverConf(p); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(WebserverConfFile(p)); string(b) != body {
			t.Fatalf("%s: %q", name, b)
		}
	}
}

func TestWriteWebserverConfSwitch(t *testing.T) {
	p := confProject(t, "nginx")
	writeConf(t, p, "mine\n")
	p.Webserver = "apache"
	if err := writeWebserverConf(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(WebserverConfFile(p)); !strings.HasPrefix(string(b), templates.GeneratedMarker) {
		t.Fatalf("apache conf: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(p.Root, ".lodev", "nginx-webserver.conf")); string(b) != "mine\n" {
		t.Fatalf("nginx conf changed: %q", b)
	}
}

func TestWriteLodevGitignore(t *testing.T) {
	p := confProject(t, "nginx")
	want := "# Generated by lodev on every `lodev start`; do not edit.\n/lodev.compose-base.yml\n/lodev.compose-full.yml\n/web-build/\n" +
		"# each <ws>-webserver.conf below is listed only while it carries the marker\n/nginx-webserver.conf\n/apache-webserver.conf\n"
	gi := filepath.Join(p.Root, ".lodev", ".gitignore")
	if err := writeLodevGitignore(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(gi); string(b) != want {
		t.Fatalf("%q", b)
	}
	writeConf(t, p, "mine\n")
	if err := writeLodevGitignore(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(gi); string(b) != strings.Replace(want, "/nginx-webserver.conf\n", "", 1) {
		t.Fatalf("%q", b)
	}
	if _, err := os.Stat(filepath.Join(p.Root, ".gitignore")); err == nil {
		t.Fatal("root .gitignore created")
	}
}

func TestNewProjectConfigEmptyNameHint(t *testing.T) {
	_, err := newProjectConfig(filepath.Join(t.TempDir(), "___"), "", "php", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("want a --name hint, got %v", err)
	}
}

func TestNewProjectConfigDocroot(t *testing.T) {
	for _, c := range []struct{ typ, in, want string }{
		{"php", ".", ""}, {"php", "web", "web"}, {"laravel", "", "public"}, {"laravel", ".", ""},
	} {
		p, err := newProjectConfig(t.TempDir(), "x", c.typ, "", "", c.in)
		if err != nil || p.Docroot != c.want {
			t.Fatalf("%s %q: got %q, %v want %q", c.typ, c.in, p.Docroot, err, c.want)
		}
	}
}

func TestScaffold(t *testing.T) {
	for ws, dir := range map[string]string{"nginx": "nginx", "apache": "apache", "none": ""} {
		root := t.TempDir()
		if err := scaffold(root, ws); err != nil {
			t.Fatal(err)
		}
		files := []string{".lodev/web-images/Dockerfile.example", ".lodev/php/example.ini.example", ".lodev/example.compose.yml.example"}
		if dir != "" {
			files = append(files, ".lodev/"+dir+"/example.conf.example")
		}
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(root, f)); err != nil {
				t.Fatalf("%s: %v", ws, err)
			}
		}
		for _, other := range []string{"nginx", "apache"} {
			if other != dir {
				if _, err := os.Stat(filepath.Join(root, ".lodev", other)); err == nil {
					t.Fatalf("%s: unexpected %s dir", ws, other)
				}
			}
		}
	}
	root := t.TempDir()
	scaffold(root, "nginx")
	f := filepath.Join(root, ".lodev", "php", "example.ini.example")
	os.WriteFile(f, []byte("mine"), 0o644)
	scaffold(root, "nginx")
	if b, _ := os.ReadFile(f); string(b) != "mine" {
		t.Fatalf("overwritten: %q", b)
	}
}

func TestCreateNoAsk(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "artisan"), nil, 0o644)
	t.Chdir(root)
	rootCmd.SetArgs([]string{"create", "--no-ask", "--docroot", "."})
	defer rootCmd.SetArgs(nil)
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	p, err := config.LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != "laravel" || p.Docroot != "" || p.PHP != "8.3" || p.Webserver != "nginx" {
		t.Fatalf("got %+v", p)
	}
	for _, f := range []string{".gitignore", "web-images/Dockerfile.example"} {
		if _, err := os.Stat(filepath.Join(root, ".lodev", f)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSettleAnswers(t *testing.T) {
	p, _ := newProjectConfig(t.TempDir(), "old", "php", "", "", "")
	p.Name = "shop"
	settleAnswers(p, "old", "php", false)
	p.ApplyDefaults()
	if len(p.Domains) != 1 || p.Domains[0] != "shop.test" {
		t.Fatalf("domains %v", p.Domains)
	}

	p, _ = newProjectConfig(t.TempDir(), "old", "php", "", "", "")
	p.Domains = []string{"custom.test"}
	p.Name = "shop"
	settleAnswers(p, "old", "php", false)
	if p.Domains[0] != "custom.test" {
		t.Fatalf("custom domain lost: %v", p.Domains)
	}

	p, _ = newProjectConfig(t.TempDir(), "x", "php", "", "", "")
	p.Type = "laravel"
	settleAnswers(p, "x", "php", false)
	if p.Docroot != "public" || !slices.Equal(p.Services, defaultServices["laravel"]) {
		t.Fatalf("got %q %v", p.Docroot, p.Services)
	}

	p, _ = newProjectConfig(t.TempDir(), "x", "php", "", "", "web")
	p.Type = "laravel"
	settleAnswers(p, "x", "php", true)
	if p.Docroot != "web" {
		t.Fatalf("set docroot overridden: %q", p.Docroot)
	}

	p.Docroot = "."
	settleAnswers(p, "x", "laravel", false)
	if p.Docroot != "" {
		t.Fatalf("got %q", p.Docroot)
	}
}

func TestStartUserWarning(t *testing.T) {
	if got := userWarning("john_doe", "John.Doe"); got != `"John.Doe" is not a valid container user name; using "john_doe" (set user: in .lodev/config.yaml to choose another)` {
		t.Fatal(got)
	}
	if got := userWarning("kuroko", ""); got != "" {
		t.Fatal(got)
	}
}

func TestWaitForUser(t *testing.T) {
	defer func(r func(*config.Project, string) bool, i time.Duration) { userReady, userPoll = r, i }(userReady, userPoll)
	userPoll = time.Millisecond
	p := &config.Project{Name: "shop"}
	n := 0
	userReady = func(*config.Project, string) bool { n++; return n == 3 }
	if err := waitForUser(p, "kuroko", time.Second); err != nil || n != 3 {
		t.Fatalf("%v %d", err, n)
	}
	userReady = func(*config.Project, string) bool { return false }
	err := waitForUser(p, "kuroko", 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "docker logs lodev-shop-web") {
		t.Fatal(err)
	}
}
