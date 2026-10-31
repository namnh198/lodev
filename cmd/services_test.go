package cmd

import (
	"slices"
	"testing"

	"lodev/internal/config"
)

func TestPortsToCheck(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	g, _ := config.LoadGlobal()
	if got := PortsToCheck(g, true); len(got) != 0 {
		t.Fatalf("running stack must not be checked: %v", got)
	}
	if got := PortsToCheck(g, false); !slices.Equal(got, []int{80, 443, 8080}) {
		t.Fatal(got)
	}
	g.Ports.MySQL = "3306"
	if got := PortsToCheck(g, false); !slices.Equal(got, []int{80, 443, 8080, 3306}) {
		t.Fatal(got)
	}
}
