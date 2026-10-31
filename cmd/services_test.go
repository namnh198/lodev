package cmd

import (
	"net"
	"slices"
	"testing"

	"lodev/internal/config"
)

func TestCheckPortReportsPortInUse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := checkPort(l.Addr().(*net.TCPAddr).Port); err == nil {
		t.Fatal("busy port not reported")
	}
}

func TestCheckPortReportsWildcardListener(t *testing.T) {
	// A listener on all interfaces (e.g. another dev proxy on :443) must count as a conflict.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := checkPort(l.Addr().(*net.TCPAddr).Port); err == nil {
		t.Fatal("wildcard listener not reported")
	}
}

func TestCheckPortFree(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if err := checkPort(port); err != nil {
		t.Fatal(err)
	}
}

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
