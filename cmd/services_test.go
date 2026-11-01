package cmd

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"lodev/internal/config"
	"lodev/internal/registry"
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

const composeLabel = "label=com.docker.compose.project=lodev-"

// stubDocker records dockerRun calls, answers the running-projects `docker ps` with running,
// a per-project `docker ps -aq` with "id-<name>" and `docker network ls -q` with "net-<name>".
func stubDocker(t *testing.T, running string) *[]string {
	t.Helper()
	oldRun, oldOut := dockerRun, dockerOutput
	t.Cleanup(func() { dockerRun, dockerOutput = oldRun, oldOut })
	var calls []string
	dockerRun = func(a ...string) error { calls = append(calls, strings.Join(a, " ")); return nil }
	dockerOutput = func(a ...string) (string, error) {
		name := strings.TrimPrefix(a[len(a)-1], composeLabel)
		switch {
		case slices.Equal(a[:2], []string{"ps", "-aq"}):
			return "id-" + name, nil
		case slices.Equal(a[:3], []string{"network", "ls", "-q"}):
			return "net-" + name, nil
		case a[0] == "ps":
			return running, nil
		}
		return "", nil
	}
	return &calls
}

func TestPoweroff(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	registry.Registry{"a": {Path: t.TempDir()}, "b": {Path: t.TempDir()}}.Save()
	os.MkdirAll(filepath.Dir(servicesFile()), 0o755)
	os.WriteFile(servicesFile(), nil, 0o644)
	calls := stubDocker(t, "lodev-b-web\nlodev-x-web")
	if err := poweroff(); err != nil {
		t.Fatal(err)
	}
	// By label, not `compose -p … stop`: compose would read a docker-compose.yml in the cwd
	// and silently skip containers whose service is not in it.
	// Containers (stopped ones too) and the project network are removed, like compose down:
	// a stale network or container breaks the next start.
	want := []string{
		"stop id-a", "rm id-a", "network rm net-a",
		"stop id-b", "rm id-b", "network rm net-b",
		"stop id-x", "rm id-x", "network rm net-x",
		"compose -p lodev-services -f " + servicesFile() + " down",
	}
	if !slices.Equal(*calls, want) {
		t.Errorf("calls:\n%s", strings.Join(*calls, "\n"))
	}
}

func TestPoweroffNoServicesFile(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	registry.Registry{"b": {Path: t.TempDir()}}.Save()
	calls := stubDocker(t, "")
	if err := poweroff(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*calls, []string{"stop id-b", "rm id-b", "network rm net-b"}) {
		t.Errorf("calls: %v", *calls)
	}
}
