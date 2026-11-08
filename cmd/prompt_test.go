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
	if p.Name != "shop" || p.Type != "laravel" || p.PHP != "8.4" || p.Docroot != "public" || p.Webserver != "nginx" {
		t.Fatalf("%+v", p)
	}
	mustContain(t, out.String(),
		"Project name (shop): ",
		"Project type [laravel, wordpress, magento, php] (laravel): ",
		"PHP version [7.4, 8.0, 8.1, 8.2, 8.3, 8.4, 8.5] (8.4): ",
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

func TestAskProjectSelectsTypeAndWebserver(t *testing.T) {
	oldUse, oldSel := useSelect, selectFn
	t.Cleanup(func() { useSelect, selectFn = oldUse, oldSel })
	useSelect = func(io.Reader) bool { return true }
	var asked []string
	selectFn = func(_ io.Reader, _ io.Writer, title string, opts []string, def string) (string, error) {
		asked = append(asked, title+"|"+strings.Join(opts, ",")+"|"+def)
		if title == "Project type" {
			return "magento", nil
		}
		return "apache", nil
	}
	p := newAskProject(t)
	var out bytes.Buffer
	// name, php and docroot still read lines
	if err := askProject(strings.NewReader("\n\n\n"), &out, p, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"Project type|laravel,wordpress,magento,php|laravel", "Webserver|nginx,apache,none|nginx"}
	if strings.Join(asked, " ") != strings.Join(want, " ") {
		t.Errorf("selects %q", asked)
	}
	if p.Type != "magento" || p.Webserver != "apache" || p.Docroot != "pub" {
		t.Errorf("%+v", p)
	}
	if strings.Contains(out.String(), "Project type [") {
		t.Errorf("type also asked as a line:\n%s", out.String())
	}
}

func TestSelectModel(t *testing.T) {
	m := newSelectModel("Webserver", []string{"nginx", "apache", "none"}, "apache")
	if m.cursor != 1 {
		t.Fatalf("cursor starts at the default: %d", m.cursor)
	}
	for _, k := range []string{"down", "down", "j", "up"} { // down stops at the last option, up comes back
		m = m.press(k)
	}
	if m.cursor != 1 {
		t.Errorf("cursor %d", m.cursor)
	}
	m = m.press("enter")
	if !m.done || m.choice != "apache" || m.view() != "" {
		t.Errorf("enter: %+v %q", m, m.view())
	}
	if c := newSelectModel("T", []string{"a", "b"}, "a").press("ctrl+c"); !c.aborted {
		t.Error("ctrl+c does not abort")
	}
	if v := newSelectModel("T", []string{"a", "b"}, "a").view(); !strings.Contains(v, "> a") || !strings.Contains(v, "  b") {
		t.Errorf("view %q", v)
	}
}
