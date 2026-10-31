package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
)

// projectDir makes a dir that looks like an initialized lodev project.
func projectDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, ".lodev"), 0o755)
	os.WriteFile(filepath.Join(d, ".lodev", "config.yaml"), nil, 0o644)
	return d
}

func project(name, root string, domains ...string) *config.Project {
	return &config.Project{Name: name, Root: root, Webserver: "nginx", Domains: domains}
}

func TestConflictSameNameOtherPath(t *testing.T) {
	a := projectDir(t)
	r := Registry{"shop": {Path: a, Domains: []string{"shop.test"}}}
	err := r.Conflict(project("shop", projectDir(t), "shop.test"))
	if err == nil || !strings.Contains(err.Error(), a) {
		t.Fatal(err)
	}
}

func TestConflictStalePathTakenOver(t *testing.T) {
	r := Registry{"shop": {Path: t.TempDir(), Domains: []string{"shop.test"}}}
	if err := r.Conflict(project("shop", projectDir(t), "shop.test")); err != nil {
		t.Fatal(err)
	}
}

func TestConflictDomain(t *testing.T) {
	a := projectDir(t)
	r := Registry{"a": {Path: a, Domains: []string{"x.test"}}}
	err := r.Conflict(project("b", projectDir(t), "x.test"))
	if err == nil || !strings.Contains(err.Error(), `"a"`) || !strings.Contains(err.Error(), a) {
		t.Fatal(err)
	}
}

func TestNoConflictSelf(t *testing.T) {
	a := projectDir(t)
	r := Registry{"shop": {Path: a, Domains: []string{"shop.test"}}}
	if err := r.Conflict(project("shop", a, "shop.test")); err != nil {
		t.Fatal(err)
	}
}

func TestAllDomains(t *testing.T) {
	r := Registry{"a": {Domains: []string{"b.test", "a.test"}}, "c": {Domains: []string{"a.test"}}}
	if got := r.AllDomains(); !reflect.DeepEqual(got, []string{"a.test", "b.test"}) {
		t.Fatal(got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	empty, err := Load()
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	r := Registry{}
	r.Put(project("shop", "/p", "shop.test"))
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil || !reflect.DeepEqual(got, r) || !reflect.DeepEqual(got.Names(), []string{"shop"}) {
		t.Fatal(got, err)
	}
}

func TestPutDropsOldNameForSamePath(t *testing.T) {
	d := projectDir(t)
	r := Registry{"shop": {Path: d, Domains: []string{"shop.test"}}, "blog": {Path: projectDir(t)}}
	dropped := r.Put(project("shop2", d, "shop2.test"))
	if _, ok := r["shop"]; ok || r["shop2"].Path != d || len(r) != 2 || !slices.Equal(dropped, []string{"shop"}) {
		t.Fatalf("%v", r)
	}
}
