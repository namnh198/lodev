package main

import (
	"slices"
	"strings"
	"testing"
)

func TestFlags(t *testing.T) {
	got, err := flags()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 || got[0] != "--label" || !strings.HasPrefix(got[1], "lodev.hash=") || !strings.HasSuffix(got[1], "+node24") ||
		!slices.Contains(got, "PHP_VERSIONS=8.2 8.3 8.4 8.5") || !slices.Contains(got, "NODE_VERSION=24") {
		t.Fatalf("flags %q", got)
	}
}
