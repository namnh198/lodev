package cmd

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestShortcutCmd(t *testing.T) {
	cases := map[string]string{
		"composer": "composer", "npm": "npm", "node": "node",
		"artisan": "php artisan", "wp": "wp", "magento": "php bin/magento",
	}
	for name, want := range cases {
		if got := shortcutCmd(name); !slices.Equal(got, strings.Fields(want)) {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestSplitService(t *testing.T) {
	cases := []struct {
		in      []string
		svc     string
		rest    []string
		wantErr bool
	}{
		{[]string{"ls"}, "web", []string{"ls"}, false},
		{[]string{"-s", "db", "mysql"}, "db", []string{"mysql"}, false},
		{[]string{"--service", "db", "x"}, "db", []string{"x"}, false},
		{[]string{"--service=db", "x"}, "db", []string{"x"}, false},
		{[]string{"ls", "-s"}, "web", []string{"ls", "-s"}, false},
		{[]string{"-s"}, "", nil, true},
		{[]string{"-s", ""}, "", nil, true},
		{[]string{"--service="}, "", nil, true},
		{[]string{"-s", "db"}, "db", []string{}, false},
	}
	for _, c := range cases {
		svc, rest, err := splitService(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("%v: err %v", c.in, err)
			continue
		}
		if !c.wantErr && (svc != c.svc || !reflect.DeepEqual(rest, c.rest)) {
			t.Errorf("%v: got %q %v", c.in, svc, rest)
		}
	}
}
