package cmd

import (
	"errors"
	"strings"
	"testing"

	"lodev/internal/config"
	"lodev/internal/registry"
)

func TestOpenTarget(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	g, _ := config.ParseGlobal(nil)
	here := &config.Project{Name: "shop", Webserver: "nginx", Domains: []string{"shop.test", "en.test"}}
	blog, _ := newProjectConfig(t.TempDir(), "blog", "php", "", "", "")
	config.SaveProject(blog)
	mail, _ := newProjectConfig(t.TempDir(), "mailpit", "php", "", "", "")
	mail.Domains = []string{"mp.test"} // mailpit.test is reserved for the service
	config.SaveProject(mail)
	reg := registry.Registry{"blog": {Path: blog.Root}, "mailpit": {Path: mail.Root}}
	for _, c := range []struct{ target, want, err string }{
		{"", "https://shop.test", ""},
		{"blog", "https://blog.test", ""},
		{"phpmyadmin", "https://phpmyadmin.test", ""},
		{"traefik", "http://localhost:10999", ""},
		{"rabbitmq", "", "lodev service enable rabbitmq"},
		{"mailpit", "https://mp.test", ""}, // the project named mailpit wins over the service
		{"nope", "", `unknown target "nope"`},
	} {
		got, err := openTarget(c.target, reg, g, here)
		if c.err == "" && (err != nil || got != c.want) || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%q: %q %v", c.target, got, err)
		}
	}
	none := &config.Project{Name: "cli", Webserver: "none"}
	if _, err := openTarget("", reg, g, none); err == nil || err.Error() != "project cli has no web URL (webserver: none)" {
		t.Errorf("none: %v", err)
	}
	if _, err := openTarget("", reg, g, nil); err == nil {
		t.Error("no project and no target must fail")
	}
}

func TestOpenCommand(t *testing.T) {
	newShop(t)
	old := openURL
	t.Cleanup(func() { openURL = old })
	var opened []string
	openURL = func(u string) error { opened = append(opened, u); return nil }
	out, _, err := runConfig(t, newOpenCmd, "--print")
	if err != nil || out != "https://shop.test\n" || len(opened) != 0 {
		t.Errorf("--print: %q %v %v", out, opened, err)
	}
	if _, _, err := runConfig(t, newOpenCmd); err != nil || len(opened) != 1 || opened[0] != "https://shop.test" {
		t.Errorf("open: %v %v", opened, err)
	}
	openURL = func(string) error { return errors.New("no browser") }
	out, _, err = runConfig(t, newOpenCmd)
	if err != nil || !strings.Contains(out, "https://shop.test") || !strings.Contains(out, "no browser") {
		t.Errorf("opener failure: %q %v", out, err)
	}
}
