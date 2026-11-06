package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"lodev/internal/config"
)

func newAskProject(t *testing.T) *config.Project {
	p, err := newProjectConfig(t.TempDir(), "shop", "laravel", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAskProjectDefaults(t *testing.T) {
	p := newAskProject(t)
	var out bytes.Buffer
	if err := askProject(strings.NewReader("\n\n\n\n\n"), &out, p, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if p.Name != "shop" || p.Type != "laravel" || p.PHP != "8.3" || p.Docroot != "public" || p.Webserver != "nginx" {
		t.Fatalf("%+v", p)
	}
	mustContain(t, out.String(),
		"Project name (shop): ",
		"Project type [laravel, wordpress, magento, php] (laravel): ",
		"PHP version [7.4, 8.0, 8.1, 8.2, 8.3, 8.4, 8.5] (8.3): ",
		`Document root, relative to the project; "." for the project root (public): `,
		"Webserver [nginx, apache, none] (nginx): ")
}

func TestAskProjectRetriesInvalid(t *testing.T) {
	p := newAskProject(t)
	var out bytes.Buffer
	in := "\nnodejs\nmagento\n 8.2 \n\n\n"
	if err := askProject(strings.NewReader(in), &out, p, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.String(), "  must be one of: laravel, wordpress, magento, php")
	// type change moves the docroot default before it is asked
	mustContain(t, out.String(), `project root (pub): `)
	if p.Type != "magento" || p.PHP != "8.2" || p.Docroot != "pub" {
		t.Fatalf("%+v", p)
	}
}

func TestAskProjectRetriesInvalidName(t *testing.T) {
	p := newAskProject(t)
	var out bytes.Buffer
	if err := askProject(strings.NewReader("Bad Name\nok\n\n\n\n\n"), &out, p, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if p.Name != "ok" || strings.Count(out.String(), "Project name") != 2 {
		t.Fatalf("%q %+v", out.String(), p)
	}
}

func TestAskProjectSkipsSetFlags(t *testing.T) {
	p := newAskProject(t)
	var out bytes.Buffer
	set := map[string]bool{"name": true, "type": true, "php": true, "docroot": true}
	if err := askProject(strings.NewReader("apache\n"), &out, p, set); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Project name") || p.Webserver != "apache" {
		t.Fatalf("%q %+v", out.String(), p)
	}
}

func TestAskProjectEOF(t *testing.T) {
	p := newAskProject(t)
	err := askProject(strings.NewReader("\n"), io.Discard, p, map[string]bool{})
	if err == nil || err.Error() != "init aborted" {
		t.Fatal(err)
	}
}

func TestAskProjectRootDocrootShowsDot(t *testing.T) {
	p, _ := newProjectConfig(t.TempDir(), "x", "php", "", "", ".")
	var out bytes.Buffer
	if err := askProject(strings.NewReader("\n\n\n\n\n"), &out, p, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	mustContain(t, out.String(), `project root (.): `)
	if p.Docroot != "" {
		t.Fatalf("docroot %q", p.Docroot)
	}
}
