package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{"My_Shop.v2": "my-shop-v2", "blog": "blog", "--A  b--": "a-b"}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Errorf("%q→%q want %q", in, got, want)
		}
	}
}

func TestDBName(t *testing.T) {
	p := &Project{Name: "my-shop-v2"}
	if p.DBName() != "my_shop_v2" {
		t.Fatal(p.DBName())
	}
}

func TestDefaults(t *testing.T) {
	p := &Project{Name: "demo"}
	p.ApplyDefaults()
	if p.Webserver != "nginx" || !reflect.DeepEqual(p.Domains, []string{"demo.test"}) {
		t.Fatalf("%+v", p)
	}
}

func valid() *Project {
	p := &Project{Name: "demo", Type: "php", PHP: "8.3", Webserver: "none", Services: []string{"mysql"}}
	p.ApplyDefaults()
	return p
}

func TestValidate(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(p *Project){
		"name":            func(p *Project) { p.Name = "Bad_Name" },
		"php":             func(p *Project) { p.PHP = "7.3" },
		"type":            func(p *Project) { p.Type = "drupal" },
		"webserver":       func(p *Project) { p.Webserver = "caddy" },
		"tld":             func(p *Project) { p.Domains = []string{"shop.local"} },
		"reserved":        func(p *Project) { p.Domains = []string{"mailpit.test"} },
		"service":         func(p *Project) { p.Services = []string{"postgres"} },
		"space":           func(p *Project) { p.Domains = []string{"github.com shop.test"} },
		"newline":         func(p *Project) { p.Domains = []string{"a.test\n1.2.3.4 bank.test"} },
		"upper":           func(p *Project) { p.Domains = []string{"Shop.test"} },
		"dash":            func(p *Project) { p.Domains = []string{"-shop.test"} },
		"bare":            func(p *Project) { p.Domains = []string{".test"} },
		"stack":           func(p *Project) { p.Name = "lodev-services" },
		"varnish none":    func(p *Project) { p.Webserver = "none"; p.Varnish = true },
		"varnish version": func(p *Project) { p.VarnishVersion = "7.x" },
	}
	for name, mutate := range bad {
		p := valid()
		mutate(p)
		if p.Validate() == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestValidateVarnish(t *testing.T) {
	for _, mutate := range []func(p *Project){
		func(p *Project) { p.Webserver, p.Varnish = "nginx", true },
		func(p *Project) { p.VarnishVersion = "9" }, // kept while Varnish is off
		func(p *Project) { p.Webserver, p.Varnish, p.VarnishVersion = "apache", true, "9.1" },
	} {
		p := valid()
		mutate(p)
		if err := p.Validate(); err != nil {
			t.Errorf("%+v: %v", p, err)
		}
	}
	p := valid()
	p.Webserver, p.Varnish = "none", true
	if err := p.Validate(); err == nil || err.Error() != "varnish needs a webserver (webserver is none)" {
		t.Errorf("err = %v", err)
	}
	p = valid()
	p.VarnishVersion = "7.x"
	if err := p.Validate(); err == nil || err.Error() != `varnish_version "7.x" is invalid (e.g. 7.6, 9)` {
		t.Errorf("err = %v", err)
	}
}

func TestVarnishImage(t *testing.T) {
	p := valid()
	if got := p.VarnishImage(); got != "varnish:7.6" {
		t.Errorf("default %q", got)
	}
	p.VarnishVersion = "9.1"
	if got := p.VarnishImage(); got != "varnish:9.1" {
		t.Errorf("override %q", got)
	}
}

func TestUnquotedPHPVersion(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
	os.WriteFile(filepath.Join(root, ".lodev", "config.yaml"),
		[]byte("name: demo\ntype: php\nphp: 8.0\nservices: [mysql]\n"), 0o644)
	p, err := LoadProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if p.PHP != "8.0" || p.Root != root {
		t.Fatalf("%+v", p)
	}
}

func TestDetectType(t *testing.T) {
	cases := map[string]string{"artisan": "laravel", "bin/magento": "magento", "wp-config.php": "wordpress", "wp-includes/": "wordpress", "wp-login.php": "wordpress", "": "php"}
	for path, want := range cases {
		d := t.TempDir()
		if path != "" {
			full := filepath.Join(d, path)
			if path[len(path)-1] == '/' {
				os.MkdirAll(full, 0o755)
			} else {
				os.MkdirAll(filepath.Dir(full), 0o755)
				os.WriteFile(full, nil, 0o644)
			}
		}
		if got := DetectType(d); got != want {
			t.Errorf("%q: got %s want %s", path, got, want)
		}
	}
}

func TestDetectTypeMagentoBeatsLaravel(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "bin"), 0o755)
	os.WriteFile(filepath.Join(d, "bin", "magento"), nil, 0o644)
	os.WriteFile(filepath.Join(d, "artisan"), nil, 0o644)
	if got := DetectType(d); got != "magento" {
		t.Errorf("got %s want magento", got)
	}
}

func TestProjectImage(t *testing.T) {
	if got := ProjectImage("shop"); got != "lodev/shop-web" {
		t.Errorf("got %s", got)
	}
}

func TestValidateCustomization(t *testing.T) {
	ok := []func(*Project){
		func(p *Project) { p.NodeJS = "20" },
		func(p *Project) { p.NodeJS = "18.19.0" },
		func(p *Project) { p.NodeJS = "lts" },
		func(p *Project) { p.NodeJS = "latest" },
		func(p *Project) { p.NodeJS = "current" },
		func(p *Project) { p.PHPExtensions = []string{"imagick", "pdo_sqlsrv"} },
		func(p *Project) { p.Packages = []string{"vim", "libstdc++6", "g++"} },
	}
	bad := []struct {
		key string
		set func(*Project)
	}{
		{"nodejs", func(p *Project) { p.NodeJS = "v20" }},
		{"nodejs", func(p *Project) { p.NodeJS = "20.x" }},
		{"php_extensions", func(p *Project) { p.PHPExtensions = []string{"imagick; rm -rf /"} }},
		{"packages", func(p *Project) { p.Packages = []string{"-vim"} }},
		{"packages", func(p *Project) { p.Packages = []string{"v"} }},
	}
	for _, f := range ok {
		p := valid()
		f(p)
		if err := p.Validate(); err != nil {
			t.Error(err)
		}
	}
	for _, c := range bad {
		p := valid()
		c.set(p)
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s: want error naming the key, got %v", c.key, err)
		}
	}
}

func TestDefaultDocroot(t *testing.T) {
	cases := map[string]string{"laravel": "public", "magento": "pub", "wordpress": "", "php": ""}
	for typ, want := range cases {
		if got := DefaultDocroot(typ); got != want {
			t.Errorf("%s: %q", typ, got)
		}
	}
}

func TestFindRootWalksUp(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".lodev"), 0o755)
	os.WriteFile(filepath.Join(root, ".lodev", "config.yaml"), nil, 0o644)
	deep := filepath.Join(root, "a", "b")
	os.MkdirAll(deep, 0o755)
	got, err := FindRoot(deep)
	if err != nil || got != root {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := FindRoot(t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestWebDomainsNone(t *testing.T) {
	p := valid()
	if p.WebDomains() != nil {
		t.Fatal(p.WebDomains())
	}
	p.Webserver = "nginx"
	if !reflect.DeepEqual(p.WebDomains(), []string{"demo.test"}) {
		t.Fatal(p.WebDomains())
	}
}

func TestSaveLoadProject(t *testing.T) {
	p := valid()
	p.Root = t.TempDir()
	if err := SaveProject(p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProject(p.Root)
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestBundledPHPIsSupported(t *testing.T) {
	for _, v := range BundledPHP {
		if !slices.Contains(SupportedPHP, v) {
			t.Errorf("bundled PHP %s is not supported", v)
		}
	}
}

func TestPHPImage(t *testing.T) {
	for ver, want := range map[string]string{
		"8.2": "lodev/webserver:dev", "8.5": "lodev/webserver:dev", "7.4": "lodev/webserver:dev-legacy-7.4", "8.1": "lodev/webserver:dev-legacy-8.1",
	} {
		if got := PHPImage(ver); got != want {
			t.Errorf("PHPImage(%q) = %q, want %q", ver, got, want)
		}
	}
}

func TestValidateAcceptsSubdomains(t *testing.T) {
	p := valid()
	p.Domains = []string{"store-en.test", "a.b.test", "x1.test"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeUser(t *testing.T) {
	cases := map[string]string{
		"kuroko": "kuroko", "John.Doe": "john_doe", "Kuroko": "kuroko", "1abc": "_1abc", "-x": "_-x",
		"nguyễn": "nguy_n", "": "lodev", "root": "root_user", "www-data": "www-data_user",
		"daemon": "daemon_user", "nobody": "nobody_user",
		strings.Repeat("a", 40): strings.Repeat("a", 32),
	}
	for in, want := range cases {
		got := SanitizeUser(in)
		if got != want {
			t.Errorf("%q→%q want %q", in, got, want)
		}
		if !userRe.MatchString(got) || got == "root" || got == "www-data" {
			t.Errorf("%q→%q is not a valid user name", in, got)
		}
	}
}

func TestContainerUser(t *testing.T) {
	old := currentUsername
	t.Cleanup(func() { currentUsername = old })
	currentUsername = func() (string, error) { return "John.Doe", nil }
	cases := []struct{ user, name, from string }{
		{"alice", "alice", ""},
		{"Bad.Name", "bad_name", "Bad.Name"},
		{"", "john_doe", "John.Doe"},
	}
	for _, c := range cases {
		name, from := (&Project{User: c.user}).ContainerUser()
		if name != c.name || from != c.from {
			t.Errorf("User %q → (%q, %q) want (%q, %q)", c.user, name, from, c.name, c.from)
		}
	}
	currentUsername = func() (string, error) { return "", errors.New("boom") }
	if name, from := (&Project{}).ContainerUser(); name != "lodev" || from != "" {
		t.Errorf("error → (%q, %q)", name, from)
	}
}

func TestSaveProjectOmitsUser(t *testing.T) {
	p := valid()
	p.Root = t.TempDir()
	if err := SaveProject(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(ProjectPath(p.Root))
	if strings.Contains(string(data), "user:") {
		t.Fatalf("user: written:\n%s", data)
	}
}

func TestProjectDB(t *testing.T) {
	for svcs, want := range map[string]string{"mysql redis": "mysql", "mariadb": "mariadb", "redis": ""} {
		p := &Project{Services: strings.Fields(svcs)}
		if got := p.DB(); got != want {
			t.Errorf("%q: %q", svcs, got)
		}
	}
	p := &Project{Name: "shop", Type: "php", PHP: "8.3", Webserver: "nginx", Services: []string{"mysql", "mariadb"}}
	p.ApplyDefaults()
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "use either mysql or mariadb, not both") {
		t.Fatal(err)
	}
}

func TestParseProjectValidationError(t *testing.T) {
	p, err := ParseProject("/r", []byte("name: x\ntype: php\nphp: \"9.9\"\n"))
	var ve *ValidationError
	if !errors.As(err, &ve) || p == nil || p.PHP != "9.9" || p.Root != "/r" || p.Webserver != "nginx" {
		t.Fatalf("got %+v, %v", p, err)
	}
	if p, err := ParseProject("/r", []byte("name: [")); err == nil || p != nil || errors.As(err, &ve) {
		t.Fatalf("yaml error: %+v, %v", p, err)
	}
}

func TestDocrootMustStayInProject(t *testing.T) {
	base := Project{Name: "x", Type: "php", PHP: "8.3", Webserver: "nginx", Domains: []string{"x.test"}}
	for _, d := range []string{"/abs", "..", "../x", "a/../../x"} {
		p := base
		p.Docroot = d
		if err := p.Validate(); err == nil || err.Error() != fmt.Sprintf("docroot %q must be a folder inside the project", d) {
			t.Errorf("%q: %v", d, err)
		}
	}
	for _, d := range []string{"public", "web/sub", ""} {
		p := base
		p.Docroot = d
		if err := p.Validate(); err != nil {
			t.Errorf("%q: %v", d, err)
		}
	}
}

func TestValidateReservedDomains(t *testing.T) {
	p := &Project{Name: "demo", Type: "php", PHP: "8.3", Webserver: "nginx", Domains: []string{"mongo.test"}}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), `domain "mongo.test" is reserved by lodev`) {
		t.Errorf("err = %v", err)
	}
}

func TestPHPImageUsesImageTag(t *testing.T) {
	defer func(old string) { ImageTag = old }(ImageTag)
	ImageTag = "v0.1.0"
	if got := PHPImage("8.3"); got != "lodev/webserver:v0.1.0" {
		t.Errorf("PHPImage(8.3) = %q", got)
	}
	if got := PHPImage("7.4"); got != "lodev/webserver:v0.1.0-legacy-7.4" {
		t.Errorf("PHPImage(7.4) = %q", got)
	}
}

func TestCronLines(t *testing.T) {
	jobs := map[string]string{
		"magento":   `* * * * * php bin/magento cron:run 2>&1 | grep -v "Ran jobs by schedule" >> var/log/magento.cron.log || true`,
		"laravel":   `* * * * * php artisan schedule:run >> storage/logs/schedule.log 2>&1`,
		"wordpress": `* * * * * wp cron event run --due-now >> wp-content/cron.log 2>&1`,
	}
	for typ, job := range jobs {
		p := &Project{Type: typ, Cron: true}
		if got := p.CronLines(); !slices.Equal(got, []string{job}) {
			t.Errorf("%s: %q", typ, got)
		}
	}
	if got := (&Project{Type: "php", Cron: true}).CronLines(); got != nil {
		t.Errorf("php default: %q", got)
	}
	custom := []string{"*/5 * * * * php x.php"}
	if got := (&Project{Type: "magento", Cron: true, CronJobs: custom}).CronLines(); !slices.Equal(got, custom) {
		t.Errorf("custom: %q", got)
	}
	if got := (&Project{Type: "magento", CronJobs: custom}).CronLines(); got != nil {
		t.Errorf("cron off: %q", got)
	}
}

func TestValidateCron(t *testing.T) {
	base := Project{Name: "x", Type: "php", PHP: "8.3", Webserver: "nginx"}
	for _, c := range []struct {
		cron bool
		typ  string
		jobs []string
		want string
	}{
		{true, "php", nil, "cron needs cron_jobs (type php has no default job)"},
		{true, "php", []string{"* * * * *"}, `cron_jobs[0] "* * * * *" needs a schedule and a command`},
		{true, "php", []string{"@hourly"}, `cron_jobs[0] "@hourly" needs a schedule and a command`},
		{true, "php", []string{"@hourly php x.php"}, ""},
		{true, "php", []string{"*/5 * * * * php x.php"}, ""},
		{false, "php", []string{"* * *"}, `cron_jobs[0] "* * *" needs a schedule and a command`},
		{true, "magento", nil, ""},
	} {
		p := base
		p.Cron, p.Type, p.CronJobs = c.cron, c.typ, c.jobs
		err := p.Validate()
		if c.want == "" && err != nil || c.want != "" && (err == nil || err.Error() != c.want) {
			t.Errorf("%+v: err = %v, want %q", c, err, c.want)
		}
	}
}

func TestCheckEnvKey(t *testing.T) {
	for _, k := range []string{"APP_KEY", "_x", "DB_HOST"} {
		if err := CheckEnvKey(k); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
	for _, k := range []string{"1A", "A-B", ""} {
		want := fmt.Sprintf("environment key %q is invalid (A-Z, a-z, 0-9, _; not starting with a digit)", k)
		if err := CheckEnvKey(k); err == nil || err.Error() != want {
			t.Errorf("%q: %v", k, err)
		}
	}
	if err := CheckEnvKey("LODEV_USER"); err == nil || err.Error() != "environment: LODEV_USER is reserved for lodev" {
		t.Errorf("LODEV_USER: %v", err)
	}
}

func TestValidateEnvironment(t *testing.T) {
	p := Project{Name: "x", Type: "php", PHP: "8.3", Webserver: "nginx", Environment: map[string]string{"LODEV_X": "1"}}
	if err := p.Validate(); err == nil || err.Error() != "environment: LODEV_X is reserved for lodev" {
		t.Errorf("err = %v", err)
	}
}

func TestDomainHintOnlyForAnotherTLD(t *testing.T) {
	writeGlobal(t, "")
	p := &Project{Name: "x", Type: "php", PHP: "8.3", Webserver: "nginx", Domains: []string{"My_shop.test"}}
	if err := p.Validate(); err == nil || strings.Contains(err.Error(), "--from-tld") {
		t.Errorf("same TLD got a move hint: %v", err)
	}
}

func BenchmarkValidateDomains(b *testing.B) {
	p := &Project{Name: "x", Type: "php", PHP: "8.3", Webserver: "nginx", Domains: []string{"a.test", "b.test", "c.test"}}
	for b.Loop() {
		p.ValidateTLD("test")
	}
}

func TestWorkDirAndIDEServerName(t *testing.T) {
	p := Project{Name: "shop"}
	if p.WorkDir() != "/app" || p.IDEServerName() != "shop" {
		t.Errorf("defaults: %q %q", p.WorkDir(), p.IDEServerName())
	}
	p.WorkingDir, p.PHPIDEConfig = "/var/www/html", "shop-ide"
	if p.WorkDir() != "/var/www/html" || p.IDEServerName() != "shop-ide" {
		t.Errorf("set: %q %q", p.WorkDir(), p.IDEServerName())
	}
}

func TestValidateWorkingDir(t *testing.T) {
	base := Project{Name: "x", Type: "php", PHP: "8.3", Webserver: "nginx", Domains: []string{"x.test"}}
	for _, d := range []string{"/app", "/var/www/html", "/srv/my-app_2"} {
		p := base
		p.WorkingDir = d
		if err := p.Validate(); err != nil {
			t.Errorf("%q: %v", d, err)
		}
	}
	for _, d := range []string{"app", "/", "/app/", "/a/../b", "/my app", "/a:b", "/etc", "/usr/local/app", "/home/x", "/tmp"} {
		p := base
		p.WorkingDir = d
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "working_dir") {
			t.Errorf("%q: want a working_dir error, got %v", d, err)
		}
	}
	p := base
	p.PHPIDEConfig = "a\nb"
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "php_ide_config") {
		t.Errorf("newline in php_ide_config: %v", err)
	}
}
