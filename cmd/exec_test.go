package cmd

import (
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
