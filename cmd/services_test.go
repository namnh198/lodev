package cmd

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"syscall"
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
	if got := PortsToCheck(g, true, false); len(got) != 0 {
		t.Fatalf("running stack must not be checked: %v", got)
	}
	if got := PortsToCheck(g, false, false); !slices.Equal(got, []int{80, 443, 8080}) {
		t.Fatal(got)
	}
	g.Ports.MySQL = "3306"
	if got := PortsToCheck(g, false, false); !slices.Equal(got, []int{80, 443, 8080, 3306}) {
		t.Fatal(got)
	}
	// Traefik down but our own MySQL up: its port is ours, not a conflict.
	if got := PortsToCheck(g, false, true); !slices.Equal(got, []int{80, 443, 8080}) {
		t.Fatal(got)
	}
}

func TestWithLockExcludesOthers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	err := withLock(func() error {
		f, err := os.OpenFile(filepath.Join(home, "lock"), os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			t.Error("a second lodev got the lock")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
