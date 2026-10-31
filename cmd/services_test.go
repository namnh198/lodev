package cmd

import (
	"net"
	"os"
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

func TestPortInUseIgnoresPermissionDenied(t *testing.T) {
	// macOS lets normal users bind :80 but not 127.0.0.1:80; that is not a conflict.
	denied := &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", syscall.EACCES)}
	inUse := &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}
	if portInUse(denied) || !portInUse(inUse) {
		t.Fatal("wrong classification")
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
