package cmd

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/ui"
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
	if got := PortsToCheck(g, true, false, false, false); len(got) != 0 {
		t.Fatalf("running stack must not be checked: %v", got)
	}
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 8080}) {
		t.Fatal(got)
	}
	g.Ports.MySQL = "3306"
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 8080, 3306}) {
		t.Fatal(got)
	}
	// Traefik down but our own MySQL up: its port is ours, not a conflict.
	if got := PortsToCheck(g, false, true, false, false); !slices.Equal(got, []int{80, 443, 8080}) {
		t.Fatal(got)
	}
	g.Ports.MySQL, g.Ports.MariaDB = "", "3307"
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 8080, 3307}) {
		t.Fatal(got)
	}
	if got := PortsToCheck(g, false, false, true, false); !slices.Equal(got, []int{80, 443, 8080}) {
		t.Fatal(got)
	}
	g.Ports.MariaDB, g.Ports.MongoDB = "", "27017"
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 8080, 27017}) {
		t.Fatal(got)
	}
	// Our own MongoDB already holds its port.
	if got := PortsToCheck(g, false, false, false, true); !slices.Equal(got, []int{80, 443, 8080}) {
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
// a per-project `docker ps --format {{.Names}}` with "lodev-<name>-web" and "lodev-<name>-varnish",
// and `docker network ls -q` with "net-<name>".
func stubDocker(t *testing.T, running string) *[]string {
	t.Helper()
	oldRun, oldOut := dockerRun, dockerOutput
	t.Cleanup(func() { dockerRun, dockerOutput = oldRun, oldOut })
	var calls []string
	dockerRun = func(a ...string) error { calls = append(calls, strings.Join(a, " ")); return nil }
	dockerOutput = func(a ...string) (string, error) {
		name := strings.TrimPrefix(a[len(a)-1], composeLabel)
		switch {
		case a[0] == "stop" || a[0] == "rm" || slices.Equal(a[:2], []string{"network", "rm"}):
			// Quiet calls: docker prints each removed ID, which must not reach the terminal.
			calls = append(calls, "quiet: "+strings.Join(a, " "))
		case a[0] == "ps" && strings.HasPrefix(a[len(a)-1], composeLabel):
			return "lodev-" + name + "-web\nlodev-" + name + "-varnish", nil
		case len(a) > 2 && slices.Equal(a[:3], []string{"network", "ls", "-q"}):
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
	out, _, restore := ui.Capture(false)
	defer restore()
	if err := poweroff(); err != nil {
		t.Fatal(err)
	}
	// Stopping prints nothing else, so say what is happening.
	mustContain(t, out.String(), "Powering off lodev: stopping all projects and shared services",
		"Stopping project a", "Stopping project b", "Stopping project x", "lodev is powered off",
		"✔ Container lodev-a-web  Stopped", "✔ Container lodev-a-varnish  Stopped",
		"✔ Container lodev-a-web  Removed", "✔ Container lodev-a-varnish  Removed")
	// By label, not `compose --env-file /dev/null -p … stop`: compose would read a docker-compose.yml in the cwd
	// and silently skip containers whose service is not in it.
	// Containers (stopped ones too) and the project network are removed, like compose down:
	// a stale network or container breaks the next start.
	want := []string{
		"quiet: stop lodev-a-web lodev-a-varnish", "quiet: rm lodev-a-web lodev-a-varnish", "quiet: network rm net-a",
		"quiet: stop lodev-b-web lodev-b-varnish", "quiet: rm lodev-b-web lodev-b-varnish", "quiet: network rm net-b",
		"quiet: stop lodev-x-web lodev-x-varnish", "quiet: rm lodev-x-web lodev-x-varnish", "quiet: network rm net-x",
		"compose --env-file /dev/null -p lodev-services -f " + servicesFile() + " down",
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
	if !slices.Equal(*calls, []string{"quiet: stop lodev-b-web lodev-b-varnish", "quiet: rm lodev-b-web lodev-b-varnish", "quiet: network rm net-b"}) {
		t.Errorf("calls: %v", *calls)
	}
}

func TestMergeServicesCompose(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	dir := filepath.Dir(servicesFile())
	os.MkdirAll(dir, 0o755)
	os.WriteFile(servicesFile(), []byte("name: lodev-services\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.compose.yml"), nil, 0o644)
	defer func(o func(...string) (string, error)) { composeConfig = o }(composeConfig)
	var args []string
	composeConfig = func(a ...string) (string, error) { args = a; return "name: lodev-services", nil }

	if err := mergeServicesCompose(); err != nil {
		t.Fatal(err)
	}
	var files []string
	for i, a := range args {
		if a == "-f" {
			files = append(files, args[i+1])
		}
	}
	if want := []string{servicesFile(), filepath.Join(dir, "a.compose.yml")}; !slices.Equal(files, want) {
		t.Errorf("files %v, want %v", files, want)
	}
	if b, _ := os.ReadFile(servicesFullFile()); string(b) != "name: lodev-services\n" {
		t.Errorf("full = %q", b)
	}
	example := filepath.Join(dir, "example.compose.yml.example")
	if _, err := os.Stat(example); err != nil {
		t.Fatal("example not written:", err)
	}

	os.WriteFile(example, []byte("mine"), 0o644)
	if err := mergeServicesCompose(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(example); string(b) != "mine" {
		t.Errorf("example overwritten: %q", b)
	}

	composeConfig = func(...string) (string, error) { return "", errors.New("bad file") }
	if err := mergeServicesCompose(); err == nil {
		t.Fatal("broken custom file must fail")
	}
	if b, _ := os.ReadFile(servicesFullFile()); string(b) != "name: lodev-services\n" {
		t.Errorf("full replaced after failed merge: %q", b)
	}
}

func TestServicesComposeFile(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	calls := stubDocker(t, "")
	servicesCompose("ps")
	os.MkdirAll(filepath.Dir(servicesFullFile()), 0o755)
	os.WriteFile(servicesFullFile(), nil, 0o644)
	servicesCompose("ps")
	want := []string{
		"compose --env-file /dev/null -p lodev-services -f " + servicesFile() + " ps",
		"compose --env-file /dev/null -p lodev-services -f " + servicesFullFile() + " ps",
	}
	if !slices.Equal(*calls, want) {
		t.Errorf("calls %v", *calls)
	}
}

func TestWarnImageChange(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	defer func(o func(...string) (string, error)) { dockerOutput = o }(dockerOutput)
	dockerOutput = func(a ...string) (string, error) {
		if a[len(a)-1] == "lodev-mysql" {
			return "mysql:8.0", nil
		}
		return "", errors.New("No such object")
	}
	g, _ := config.LoadGlobal()
	for ver, want := range map[string]string{"8.4": "lodev-mysql: mysql:8.0 → mysql:8.4", "8.0": ""} {
		g.Versions["mysql"] = ver
		_, stderr, restore := ui.Capture(false)
		warnImageChange(g)
		restore()
		if want == "" && stderr.Len() != 0 || want != "" && !strings.Contains(stderr.String(), want) {
			t.Errorf("mysql %s: stderr %q", ver, stderr.String())
		}
	}
}

func TestVolumesWarningNamesAllData(t *testing.T) {
	for _, svc := range []string{"MySQL", "MariaDB", "Redis", "OpenSearch", "RabbitMQ"} {
		if !strings.Contains(volumesWarning, svc) {
			t.Errorf("--volumes warning does not mention %s: %q", svc, volumesWarning)
		}
	}
}

func TestSetServiceKeepsComments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	path := filepath.Join(home, "config.yaml")
	os.WriteFile(path, []byte("# mine\nservices:\n  redis: true\n"), 0o644)
	if err := setService("rabbitmq", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if s := string(b); !strings.Contains(s, "# mine") || !strings.Contains(s, "rabbitmq: true") || strings.Contains(s, "ports:") {
		t.Fatalf("%s", s)
	}
}

// Compose starts mongodb again for an enabled mongo-express (depends_on), so the disable would not stick.
func TestCheckDisable(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	g, _ := config.LoadGlobal()
	g.Services["mongo-express"] = true
	err := checkDisable("mongodb", g)
	if err == nil || err.Error() != "mongo-express needs mongodb: run `lodev services disable mongo-express` first" {
		t.Errorf("err = %v", err)
	}
	for _, svc := range []string{"mongo-express", "redis"} {
		if err := checkDisable(svc, g); err != nil {
			t.Errorf("%s: %v", svc, err)
		}
	}
	g.Services["mongo-express"] = false
	if err := checkDisable("mongodb", g); err != nil {
		t.Errorf("mongodb with mongo-express off: %v", err)
	}
}

type dataFix struct {
	calls     []string
	volumes   map[string]bool // named volumes that exist
	runErr    error
	container bool // lodev-<svc> containers exist
}

// stubServiceData swaps the docker seams prepareServiceData uses.
func stubServiceData(t *testing.T) *dataFix {
	t.Helper()
	f := &dataFix{volumes: map[string]bool{}, container: true}
	oldRun, oldOut := dockerRun, dockerOutput
	t.Cleanup(func() { dockerRun, dockerOutput = oldRun, oldOut })
	dockerRun = func(a ...string) error { f.calls = append(f.calls, strings.Join(a, " ")); return f.runErr }
	dockerOutput = func(a ...string) (string, error) {
		f.calls = append(f.calls, strings.Join(a, " "))
		switch {
		case a[0] == "volume" && a[1] == "inspect":
			if !f.volumes[a[2]] {
				return "", errors.New("no such volume")
			}
		case a[0] == "ps" && f.container:
			return "abc123", nil
		}
		return "", nil
	}
	return f
}

func mountGlobal(t *testing.T) *config.Global {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	g, err := config.LoadGlobal()
	if err != nil {
		t.Fatal(err)
	}
	g.MountVolume = true
	return g
}

func runs(calls []string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, "run ") || strings.HasPrefix(c, "stop ") {
			out = append(out, c)
		}
	}
	return out
}

func TestPrepareServiceDataCopies(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-mysql"] = true
	g := mountGlobal(t)
	out, _, restore := ui.Capture(false)
	defer restore()
	if err := prepareServiceData(g, []string{"mysql", "mailpit"}); err != nil {
		t.Fatal(err)
	}
	path := g.MountPath("mysql")
	want := []string{"stop lodev-mysql", "run --rm --user 0 --entrypoint cp -v lodev-mysql:/from -v " + path + ":/to mysql:8.0 -a /from/. /to/"}
	if got := runs(f.calls); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	mustContain(t, out.String(), "Copied volume lodev-mysql to "+path+" (the volume is kept)")
}

func TestPrepareServiceDataKeepsData(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-mysql"] = true
	g := mountGlobal(t)
	writeFile(t, filepath.Join(g.MountPath("mysql"), "ibdata1"), "x")
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("touched a folder with data: %v", got)
	}
}

func TestPrepareServiceDataNoVolume(t *testing.T) {
	f := stubServiceData(t)
	g := mountGlobal(t)
	if err := prepareServiceData(g, []string{"redis"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("copied without a volume: %v", got)
	}
	if fi, err := os.Stat(g.MountPath("redis")); err != nil || !fi.IsDir() {
		t.Errorf("folder not created: %v", err)
	}
}

func TestPrepareServiceDataNoContainer(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-redis"], f.container = true, false
	g := mountGlobal(t)
	if err := prepareServiceData(g, []string{"redis"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) != 1 || !strings.HasPrefix(got[0], "run ") {
		t.Errorf("calls %v", got)
	}
}

func TestPrepareServiceDataOpenSearchMode(t *testing.T) {
	stubServiceData(t)
	g := mountGlobal(t)
	if err := prepareServiceData(g, []string{"opensearch"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(g.MountPath("opensearch")); err != nil || fi.Mode().Perm() != 0o777 {
		t.Errorf("opensearch folder %v %v", fi.Mode(), err)
	}
}

func TestPrepareServiceDataOff(t *testing.T) {
	f := stubServiceData(t)
	g := mountGlobal(t)
	g.MountVolume = false
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) > 0 {
		t.Errorf("docker calls with mount_volume off: %v", f.calls)
	}
	if _, err := os.Stat(filepath.Join(g.DataPath(), "mysql")); err == nil {
		t.Error("folder created with mount_volume off")
	}
}

func TestPrepareServiceDataCopyFails(t *testing.T) {
	f := stubServiceData(t)
	f.volumes["lodev-mysql"], f.runErr = true, errors.New("boom")
	g := mountGlobal(t)
	err := prepareServiceData(g, []string{"mysql"})
	if err == nil || !strings.Contains(err.Error(), g.MountPath("mysql")) || !strings.Contains(err.Error(), "empty") {
		t.Errorf("err = %v", err)
	}
}

// On Linux a database chowns its folder to its own UID (often 0700/0750): lodev cannot read it then.
func TestPrepareServiceDataUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads anything")
	}
	f := stubServiceData(t)
	f.volumes["lodev-mysql"], f.volumes["lodev-opensearch"] = true, true
	g := mountGlobal(t)
	for _, svc := range []string{"mysql", "opensearch"} {
		writeFile(t, filepath.Join(g.MountPath(svc), "data"), "x")
		os.Chmod(g.MountPath(svc), 0o300) // not listable, like a folder owned by another UID
		defer os.Chmod(g.MountPath(svc), 0o755)
	}
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatalf("unreadable folder: %v", err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("copied into an unreadable folder: %v", got)
	}
	// An existing opensearch folder is opensearch's own: no chmod (it fails on another UID's folder).
	os.Chmod(g.MountPath("opensearch"), 0o700)
	if err := prepareServiceData(g, []string{"opensearch"}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(g.MountPath("opensearch")); fi.Mode().Perm() != 0o700 {
		t.Errorf("existing opensearch folder re-chmodded: %v", fi.Mode())
	}
}
