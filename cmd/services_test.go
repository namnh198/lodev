package cmd

import (
	"errors"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"lodev/internal/config"
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
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 10999}) {
		t.Fatal(got)
	}
	g.Ports.MySQL = "3306"
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 10999, 3306}) {
		t.Fatal(got)
	}
	// Traefik down but our own MySQL up: its port is ours, not a conflict.
	if got := PortsToCheck(g, false, true, false, false); !slices.Equal(got, []int{80, 443, 10999}) {
		t.Fatal(got)
	}
	g.Ports.MySQL, g.Ports.MariaDB = "", "3307"
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 10999, 3307}) {
		t.Fatal(got)
	}
	if got := PortsToCheck(g, false, false, true, false); !slices.Equal(got, []int{80, 443, 10999}) {
		t.Fatal(got)
	}
	g.Ports.MariaDB, g.Ports.MongoDB = "", "27017"
	if got := PortsToCheck(g, false, false, false, false); !slices.Equal(got, []int{80, 443, 10999, 27017}) {
		t.Fatal(got)
	}
	// Our own MongoDB already holds its port.
	if got := PortsToCheck(g, false, false, false, true); !slices.Equal(got, []int{80, 443, 10999}) {
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

// stubPoweroff answers poweroff's docker queries: running maps each running web container to its
// project root ("" = no folder any more); runs records the docker commands that do the work.
func stubPoweroff(t *testing.T, running map[string]string) *[]string {
	t.Helper()
	oldRun, oldOut := dockerRun, dockerOutput
	t.Cleanup(func() { dockerRun, dockerOutput = oldRun, oldOut })
	var runs []string
	dockerRun = func(a ...string) error { runs = append(runs, strings.Join(a, " ")); return nil }
	dockerOutput = func(a ...string) (string, error) {
		switch {
		case a[0] == "ps" && slices.Contains(a, "name=^lodev-.*-web$"):
			names := slices.Sorted(maps.Keys(running))
			return strings.Join(names, "\n"), nil
		case a[0] == "inspect":
			root := running[a[len(a)-1]]
			if root == "" {
				return "/gone/.lodev", nil
			}
			return filepath.Join(root, ".lodev"), nil
		case a[0] == "ps" && slices.Contains(a, "-aq"):
			return "id1\nid2", nil
		}
		t.Fatalf("unexpected docker %q", a)
		return "", nil
	}
	return &runs
}

func TestPoweroff(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	os.MkdirAll(filepath.Dir(servicesFile()), 0o755)
	os.WriteFile(servicesFile(), nil, 0o644)
	a, _ := newProjectConfig(t.TempDir(), "a", "php", "", "", "")
	config.SaveProject(a)
	runs := stubPoweroff(t, map[string]string{"lodev-a-web": a.Root, "lodev-gone-web": ""})
	writeFile(t, projectRouteFile("a"), "x")
	writeFile(t, projectRouteFile("gone"), "x")
	out, _, restore := ui.Capture(false)
	defer restore()
	if err := poweroff(); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(config.TraefikDir(), "p-*.yaml")); len(left) > 0 {
		t.Errorf("route files kept: %v", left)
	}
	mustContain(t, out.String(), "Powering off lodev", "Stopping project a", "Stopping project gone", "lodev is powered off")
	if strings.Contains(out.String(), "✔ Container") {
		t.Errorf("lodev printed its own container lines:\n%s", out.String())
	}
	want := []string{
		// compose itself stops and removes the project and reports each container.
		"compose --env-file /dev/null -p lodev-a -f " + composeFile(a) + " down",
		// No folder, no compose file: remove its containers by compose's project label.
		"rm -f id1 id2",
		"compose --env-file /dev/null -p lodev-services -f " + servicesFile() + " down",
	}
	if !slices.Equal(*runs, want) {
		t.Errorf("runs:\n%s", strings.Join(*runs, "\n"))
	}
}

func TestPoweroffNoServicesFile(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	runs := stubPoweroff(t, nil)
	if err := poweroff(); err != nil {
		t.Fatal(err)
	}
	if len(*runs) != 0 {
		t.Errorf("runs: %v", *runs)
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
	if b, _ := os.ReadFile(servicesFullFile()); string(b) != "# generated by lodev. Do NOT change anything\n# Merged from services.compose-base.yaml and the *.compose.yml files here: edit those instead.\nname: lodev-services\n" {
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
	if b, _ := os.ReadFile(servicesFullFile()); string(b) != "# generated by lodev. Do NOT change anything\n# Merged from services.compose-base.yaml and the *.compose.yml files here: edit those instead.\nname: lodev-services\n" {
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
	if err := setServices([]string{"rabbitmq"}, true); err != nil {
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
	err := checkDisable([]string{"mongodb"}, g)
	if err == nil || err.Error() != "mongo-express needs mongodb: run `lodev service disable mongo-express` first" {
		t.Errorf("err = %v", err)
	}
	for _, svc := range []string{"mongo-express", "redis"} {
		if err := checkDisable([]string{svc}, g); err != nil {
			t.Errorf("%s: %v", svc, err)
		}
	}
	if err := checkDisable([]string{"mongodb", "mongo-express"}, g); err != nil {
		t.Errorf("both at once: %v", err)
	}
	g.Services["mongo-express"] = false
	if err := checkDisable([]string{"mongodb"}, g); err != nil {
		t.Errorf("mongodb with mongo-express off: %v", err)
	}
}

type dataFix struct {
	calls     []string
	volumes   map[string]bool // named volumes that exist
	runErr    error
	container bool   // lodev-<svc> containers exist
	mounts    string // what `docker inspect` says is on the container's data folder: "<type> <source>"
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
		case a[0] == "inspect":
			if !f.container {
				return "", errors.New("no such object")
			}
			return f.mounts, nil
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
	g.MountPaths = map[string]string{}
	for _, s := range config.DataServices {
		g.MountPaths[s] = s
	}
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
	f.mounts = "volume /var/lib/docker/volumes/lodev-mysql/_data"
	g := mountGlobal(t)
	g.MountPaths = nil
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("docker runs without a mount path: %v", got)
	}
	if _, err := os.Stat(filepath.Join(g.DataPath(), "mysql")); err == nil {
		t.Error("folder created without a mount path")
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

// offGlobal is mountGlobal after mysql's mount path was removed, its container still on the folder.
func offGlobal(t *testing.T, f *dataFix) (*config.Global, string) {
	t.Helper()
	g := mountGlobal(t)
	path := filepath.Join(t.TempDir(), "my data")
	writeFile(t, filepath.Join(path, "ibdata1"), "x")
	delete(g.MountPaths, "mysql")
	f.mounts = "bind " + path
	return g, path
}

func TestPrepareServiceDataCopiesBack(t *testing.T) {
	f := stubServiceData(t)
	g, path := offGlobal(t, f)
	out, _, restore := ui.Capture(false)
	defer restore()
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"stop lodev-mysql", "run --rm --user 0 --entrypoint sh -v " + path + ":/from -v lodev-mysql:/to mysql:8.0 -c find /to -mindepth 1 -delete && cp -a /from/. /to/"}
	if got := runs(f.calls); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !slices.Contains(f.calls, `inspect -f {{range .Mounts}}{{if eq .Destination "/var/lib/mysql"}}{{.Type}} {{.Source}}{{end}}{{end}} lodev-mysql`) {
		t.Errorf("data folder not inspected: %v", f.calls)
	}
	kept, _ := filepath.Glob(path + ".copied-*")
	if _, err := os.Stat(path); err == nil || len(kept) != 1 {
		t.Fatalf("folder not renamed: %v", kept)
	}
	if _, err := os.Stat(filepath.Join(kept[0], "ibdata1")); err != nil {
		t.Errorf("data not kept: %v", err)
	}
	mustContain(t, out.String(), "Copied "+path+" back to volume lodev-mysql (the folder is kept as "+kept[0]+")")
}

// Without its container lodev cannot tell which folder the data was in: nothing to copy.
func TestPrepareServiceDataCopyBackNoContainer(t *testing.T) {
	f := stubServiceData(t)
	g, path := offGlobal(t, f)
	f.container = false
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("calls %v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("folder moved: %v", err)
	}
}

func TestPrepareServiceDataCopyBackEmptyFolder(t *testing.T) {
	f := stubServiceData(t)
	g, path := offGlobal(t, f)
	os.Remove(filepath.Join(path, "ibdata1"))
	if err := prepareServiceData(g, []string{"mysql"}); err != nil {
		t.Fatal(err)
	}
	if got := runs(f.calls); len(got) > 0 {
		t.Errorf("copied an empty folder: %v", got)
	}
}

func TestPrepareServiceDataCopyBackFails(t *testing.T) {
	f := stubServiceData(t)
	g, path := offGlobal(t, f)
	f.runErr = errors.New("boom")
	err := prepareServiceData(g, []string{"mysql"})
	if err == nil || !strings.Contains(err.Error(), "copy "+path+" back to volume lodev-mysql") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "ibdata1")); err != nil {
		t.Errorf("folder touched after a failed copy: %v", err)
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

func TestSharedServiceArgs(t *testing.T) {
	got, err := sharedServiceArgs([]string{"redis", "rabbitmq", "redis"})
	if err != nil || !slices.Equal(got, []string{"redis", "rabbitmq"}) {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := sharedServiceArgs([]string{"redis", "postgres"}); err == nil || !strings.HasPrefix(err.Error(), `unknown service "postgres"`) {
		t.Errorf("unknown: %v", err)
	}
}

func TestSetServices(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	if err := setServices([]string{"rabbitmq", "mongodb"}, true); err != nil {
		t.Fatal(err)
	}
	g, _ := config.LoadGlobal()
	if !g.Services["rabbitmq"] || !g.Services["mongodb"] {
		t.Errorf("services %v", g.Services)
	}
}

func TestEnsureDBConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	if err := ensureDBConfig(); err != nil {
		t.Fatal(err)
	}
	for _, db := range []string{"mysql", "mariadb"} {
		b, err := os.ReadFile(filepath.Join(home, "services", db, "lodev.cnf"))
		if err != nil || !strings.HasPrefix(string(b), "# ") || !strings.Contains(string(b), "[mysqld]\n") ||
			!strings.Contains(string(b), "max_allowed_packet=500M") || !strings.Contains(string(b), "binlog_expire_logs_seconds=864000") || !strings.Contains(string(b), "max_binlog_size=100M") || !strings.Contains(string(b), "loose-default_authentication_plugin=mysql_native_password") {
			t.Fatalf("%s: %q %v", db, b, err)
		}
	}
	mine := filepath.Join(home, "services", "mysql", "lodev.cnf")
	os.WriteFile(mine, []byte("[mysqld]\nmax_connections=500\n"), 0o644)
	if err := ensureDBConfig(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(mine); string(b) != "[mysqld]\nmax_connections=500\n" {
		t.Errorf("user's config overwritten: %q", b)
	}
}

func TestEnsureDBConfigMovesOldFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LODEV_HOME", home)
	old := filepath.Join(home, "mariadb", "lodev.cnf")
	os.MkdirAll(filepath.Dir(old), 0o755)
	os.WriteFile(old, []byte("[mysqld]\nmine=1\n"), 0o644)
	if err := ensureDBConfig(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "services", "mariadb", "lodev.cnf")); string(b) != "[mysqld]\nmine=1\n" {
		t.Errorf("old config not moved: %q", b)
	}
	if _, err := os.Stat(filepath.Join(home, "mariadb")); err == nil {
		t.Error("old folder left behind")
	}
}

func TestServicesComposeFileNames(t *testing.T) {
	t.Setenv("LODEV_HOME", "/h")
	if servicesFile() != "/h/services/services.compose-base.yaml" || servicesFullFile() != "/h/services/services.compose-full.yaml" {
		t.Errorf("%s %s", servicesFile(), servicesFullFile())
	}
}

// Before the first `service start` of a lodev that renamed the compose files, only the old names
// exist: poweroff and service stop must still reach the running services.
func TestPoweroffOldServicesFiles(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	dir := filepath.Dir(servicesFile())
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "compose.yaml"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "compose-full.yaml"), nil, 0o644)
	runs := stubPoweroff(t, nil)
	if err := poweroff(); err != nil {
		t.Fatal(err)
	}
	want := []string{"compose --env-file /dev/null -p lodev-services -f " + filepath.Join(dir, "compose-full.yaml") + " down"}
	if !slices.Equal(*runs, want) {
		t.Errorf("runs %q, want %q", *runs, want)
	}
}

func TestServicesComposeFileOrder(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	dir := filepath.Dir(servicesFile())
	os.MkdirAll(dir, 0o755)
	if got := servicesComposeFile(); got != "" {
		t.Errorf("none: %q", got)
	}
	for _, n := range []string{"compose.yaml", "compose-full.yaml", "services.compose-base.yaml", "services.compose-full.yaml"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
		if got := servicesComposeFile(); got != filepath.Join(dir, n) {
			t.Errorf("after %s: %q", n, got)
		}
	}
}

func TestCustomServices(t *testing.T) {
	f := filepath.Join(t.TempDir(), "full.yaml")
	os.WriteFile(f, []byte("services:\n  traefik: {}\n  mysql: {}\n  syspro-forward-80: {}\n  adminer: {}\n"), 0o644)
	got, err := customServices(f)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"adminer", "syspro-forward-80"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := customServices(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("missing file must fail")
	}
}

// mergedServices is a merged services file on tld test: the four UIs as the template routes them, and a custom adminer.
const mergedServices = `services:
  mailpit: {environment: {VIRTUAL_HOST: mailpit.test, VIRTUAL_PORT: "8025"}, networks: {lodev: null}}
  phpmyadmin: {environment: {VIRTUAL_HOST: phpmyadmin.test}, networks: {lodev: null}}
  rabbitmq: {environment: {VIRTUAL_HOST: rabbitmq.test, VIRTUAL_PORT: "15672"}, networks: {lodev: null}}
  mongo-express: {environment: {VIRTUAL_HOST: mongo.test, VIRTUAL_PORT: "8081"}, networks: {lodev: null}}
  adminer: {environment: {VIRTUAL_HOST: adminer.test, VIRTUAL_PORT: "8080"}, networks: {lodev: null}}
`

func TestServicesDomainsNoRepeats(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	got, err := servicesDomains([]byte(mergedServices))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"adminer.test", "mailpit.test", "mongo.test", "phpmyadmin.test", "rabbitmq.test"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestServicesComposeUpKeepsOldCertsUntilUp(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	old := filepath.Join(config.Home(), "certs", "dynamic.yaml")
	writeFile(t, old, "old")
	defer func(o func(...string) error) { dockerRun = o }(dockerRun)
	dockerRun = func(...string) error { return errors.New("up failed") }
	if err := servicesComposeUp([]string{"up", "-d", "traefik"}); err == nil {
		t.Fatal("want the up error")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("dynamic.yaml removed although up failed: %v", err)
	}
	dockerRun = func(...string) error { return nil }
	if err := servicesComposeUp([]string{"up", "-d", "traefik"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dynamic.yaml kept after up: %v", err)
	}
}
