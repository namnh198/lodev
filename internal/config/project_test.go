package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
		"name":      func(p *Project) { p.Name = "Bad_Name" },
		"php":       func(p *Project) { p.PHP = "7.3" },
		"type":      func(p *Project) { p.Type = "drupal" },
		"webserver": func(p *Project) { p.Webserver = "caddy" },
		"tld":       func(p *Project) { p.Domains = []string{"shop.local"} },
		"reserved":  func(p *Project) { p.Domains = []string{"mailpit.test"} },
		"service":   func(p *Project) { p.Services = []string{"postgres"} },
	}
	for name, mutate := range bad {
		p := valid()
		mutate(p)
		if p.Validate() == nil {
			t.Errorf("%s: expected error", name)
		}
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
	cases := map[string]string{"artisan": "laravel", "bin/magento": "magento", "wp-config.php": "wordpress", "wp-includes/": "wordpress", "": "php"}
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
