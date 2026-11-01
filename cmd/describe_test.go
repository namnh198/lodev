package cmd

import (
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
)

func describeProject(webserver string, services ...string) *config.Project {
	return &config.Project{
		Name: "laravel", Type: "laravel", PHP: "8.4", Webserver: webserver, Docroot: "public",
		Domains: []string{"laravel.test"}, Services: services,
	}
}

func rowNamed(t *testing.T, rows [][]string, name string) []string {
	t.Helper()
	for _, r := range rows {
		if r[0] == name {
			return r
		}
	}
	t.Fatalf("no %q row in %v", name, rows)
	return nil
}

func TestDescribeRows(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	g, _ := config.LoadGlobal()
	g.Ports.MySQL = "3306"
	running := func(string) string { return "running" }

	rows := describeRows(describeProject("nginx", "mysql", "redis"), g, running, nil)
	var names []string
	for _, r := range rows {
		names = append(names, r[0])
	}
	if want := []string{"web", "mysql", "redis", "phpMyAdmin", "Mailpit", "Project URLs"}; !slices.Equal(names, want) {
		t.Fatalf("rows %v", names)
	}
	web := rowNamed(t, rows, "web")
	if web[1] != "OK" || !strings.HasPrefix(web[2], "https://laravel.test") {
		t.Errorf("web %q", web)
	}
	for _, s := range []string{"laravel PHP 8.4", "Server: nginx", "Docroot: 'public'"} {
		if !strings.Contains(web[3], s) {
			t.Errorf("web info %q lacks %q", web[3], s)
		}
	}
	db := rowNamed(t, rows, "mysql")
	if !strings.Contains(db[2], "InDocker: mysql:3306") || !strings.Contains(db[2], "Host: 127.0.0.1:3306") ||
		!strings.Contains(db[3], "mysql:8.0") || !strings.Contains(db[3], "Database: laravel") {
		t.Errorf("mysql %q", db)
	}

	stopped := describeRows(describeProject("nginx", "mysql", "redis"), g, func(string) string { return "" }, nil)
	if s := rowNamed(t, stopped, "web")[1]; s != "stopped" {
		t.Errorf("no container: STAT %q", s)
	}

	maria := describeRows(describeProject("nginx", "mariadb"), g, running, nil)
	if info := rowNamed(t, maria, "mariadb")[3]; !strings.Contains(info, "mariadb:11.4") {
		t.Errorf("mariadb info %q", info)
	}

	cli := describeRows(describeProject("none", "mysql"), g, running, nil)
	for _, r := range cli {
		if r[0] == "Project URLs" || strings.Contains(strings.Join(r, " "), "https://laravel.test") {
			t.Errorf("CLI project shows a URL: %q", r)
		}
	}

	extra := describeRows(describeProject("nginx"), g, running, []serviceInfo{{"adminer", "running", "8080/tcp", "adminer"}})
	ad := rowNamed(t, extra, "adminer")
	if ad[1] != "OK" || !strings.Contains(ad[2], "InDocker: adminer") || ad[3] != "adminer" {
		t.Errorf("adminer %q", ad)
	}
}

func TestDescribeTarget(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	saveProjects(t, []string{"a"}, nil)
	t.Chdir(t.TempDir())
	p, err := describeTarget([]string{"a"})
	if err != nil || p.Name != "a" {
		t.Fatalf("p %v, err %v", p, err)
	}
	if _, err := describeTarget([]string{"nope"}); err == nil || err.Error() != `unknown project "nope" (see lodev list)` {
		t.Fatalf("unknown: %v", err)
	}
}
