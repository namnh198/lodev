package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEntrypointGroup runs the entrypoint's user/group block in a local lodev/webserver image: a
// host GID the image already names gets the user's name (macOS staff = Debian dialout), www-data
// stays, and a new GID gets a group named after the user. Skipped without Docker or the image.
func TestEntrypointGroup(t *testing.T) {
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", "lodev/webserver").Output()
	image := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if err != nil || image == "" {
		t.Skip("needs Docker and a lodev/webserver image")
	}
	b, err := FS.ReadFile("webserver/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, `  group=$(getent group "$LODEV_GID"`)
	end := strings.Index(s[max(start, 0):], "\n  fi\n")
	if start < 0 || end < 0 {
		t.Fatal("entrypoint.sh has no group block")
	}
	block := s[start : start+end+len("\n  fi\n")]
	for _, c := range []struct {
		gid  int
		want string
	}{{20, "kuroko"}, {33, "www-data"}, {4242, "kuroko"}} {
		gid := strconv.Itoa(c.gid)
		got, err := exec.Command("docker", "run", "--rm", "--user", "0", "--entrypoint", "sh",
			"-e", "LODEV_USER=kuroko", "-e", "LODEV_GID="+gid, image,
			"-c", block+`echo "$group"; getent group `+gid+` | cut -d: -f1`).CombinedOutput()
		if err != nil {
			t.Fatalf("gid %s: %v\n%s", gid, err, got)
		}
		if lines := strings.Fields(string(got)); len(lines) != 2 || lines[0] != c.want || lines[1] != c.want {
			t.Errorf("gid %s: got %q, want %s twice", gid, got, c.want)
		}
	}
}

// TestEntrypointChownsVolumeDirs: a new volume is root-owned, so after the user is created the
// entrypoint hands each LODEV_VOLUME_DIRS top folder to the host user (not recursively).
func TestEntrypointChownsVolumeDirs(t *testing.T) {
	b, err := FS.ReadFile("webserver/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	loop := strings.Index(s, "for d in ${LODEV_VOLUME_DIRS:-}; do")
	if loop < 0 || loop < strings.Index(s, "useradd ") {
		t.Fatal("entrypoint.sh has no LODEV_VOLUME_DIRS loop after the user block")
	}
	body := s[loop : loop+strings.Index(s[loop:], "\ndone\n")]
	for _, want := range []string{`t="$LODEV_WORKDIR/$d"`, `stat -c %u "$t"`, `chown "$LODEV_UID:$LODEV_GID" "$t"`} {
		if !strings.Contains(body, want) {
			t.Errorf("loop lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "chown -R") {
		t.Error("loop must not chown recursively")
	}
}

// TestEntrypointCopiesDaemons: supervisor's daemon programs are copied in only when LODEV_DAEMONS=1.
func TestEntrypointCopiesDaemons(t *testing.T) {
	b, err := FS.ReadFile("webserver/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	want := "if [ \"${LODEV_DAEMONS:-}\" = 1 ]; then\n  cp /etc/lodev/daemons/daemons.conf /etc/supervisor/conf.d/\nfi\n"
	if !strings.Contains(string(b), want) {
		t.Errorf("entrypoint.sh lacks:\n%s", want)
	}
}

// TestEntrypointOpcacheOff runs the conf.d block in a local lodev/webserver image: with
// LODEV_OPCACHE_VALIDATE=0 both the cli and fpm conf.d link an ini that turns validate_timestamps
// off; without it no such link is left. Skipped without Docker or the image.
func TestEntrypointOpcacheOff(t *testing.T) {
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", "lodev/webserver").Output()
	image := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if err != nil || image == "" {
		t.Skip("needs Docker and a lodev/webserver image")
	}
	b, err := FS.ReadFile("webserver/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "for sapi in cli fpm; do")
	end := strings.Index(s[max(start, 0):], "\ndone\n")
	if start < 0 || end < 0 {
		t.Fatal("entrypoint.sh has no sapi loop")
	}
	block := s[start : start+end+len("\ndone\n")]
	script := "set -e\nmkdir -p /etc/lodev/php.d\n" + block +
		`for sapi in cli fpm; do cat /etc/php/$LODEV_PHP/$sapi/conf.d/99-zz-lodev-opcache-off.ini 2>/dev/null || echo none; done`
	for _, c := range []struct {
		env  []string
		want string
	}{
		{[]string{"-e", "LODEV_OPCACHE_VALIDATE=0"}, "opcache.validate_timestamps=0 opcache.validate_timestamps=0"},
		{nil, "none none"},
	} {
		args := append([]string{"run", "--rm", "--user", "0", "--entrypoint", "sh", "-e", "LODEV_PHP=8.3"}, c.env...)
		got, err := exec.Command("docker", append(args, image, "-c", script)...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", c.env, err, got)
		}
		if g := strings.Join(strings.Fields(string(got)), " "); g != c.want {
			t.Errorf("%v: got %q, want %q", c.env, g, c.want)
		}
	}
}

// entrypointBlock returns the entrypoint.sh text from start up to and including the first end after
// it, plus the local lodev/webserver image to run it in (skips without Docker or the image).
func entrypointBlock(t *testing.T, start, end string) (image, block string) {
	t.Helper()
	out, err := exec.Command("docker", "images", "--format", "{{.Repository}}:{{.Tag}}", "lodev/webserver").Output()
	image = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if err != nil || image == "" {
		t.Skip("needs Docker and a lodev/webserver image")
	}
	b, err := FS.ReadFile("webserver/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, start)
	j := strings.Index(s[max(i, 0):], end)
	if i < 0 || j < 0 {
		t.Fatalf("entrypoint.sh has no block %q", start)
	}
	return image, s[i : i+j+len(end)]
}

// caSetup gives the CA block a rootCA.pem to install and, in /etc/ssl/certs, a bundle file with two
// certificates: the kind OrbStack injects, which update-ca-certificates warns about and skips.
const caSetup = `set -e
mkdir -p /etc/lodev
c=/usr/share/ca-certificates/mozilla
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -subj /CN=lodev-test \
  -keyout /dev/null -out /etc/lodev/rootCA.pem 2>/dev/null
cat $c/ISRG_Root_X1.crt $c/ISRG_Root_X2.crt > /etc/ssl/certs/bundle.pem
`

// TestEntrypointCAQuiet: installing the local CA prints nothing at all, even with a multi-cert
// bundle file in /etc/ssl/certs that rehash warns about, and succeeds. Skipped without Docker or the image.
func TestEntrypointCAQuiet(t *testing.T) {
	image, block := entrypointBlock(t, "if [ -f /etc/lodev/rootCA.pem ]; then", "\nfi\n")
	got, err := exec.Command("docker", "run", "--rm", "--user", "0", "--entrypoint", "sh", image,
		"-c", caSetup+block).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	if len(got) != 0 {
		t.Errorf("want no output, got:\n%s", got)
	}
}

// TestEntrypointCAFailureShown: when update-ca-certificates fails, its output reaches stderr and the
// block exits non-zero, so the filter never hides a real failure.
func TestEntrypointCAFailureShown(t *testing.T) {
	image, block := entrypointBlock(t, "if [ -f /etc/lodev/rootCA.pem ]; then", "\nfi\n")
	stub := "mkdir -p /stub\nprintf '#!/bin/sh\\necho boom\\nexit 1\\n' > /stub/update-ca-certificates\nchmod +x /stub/update-ca-certificates\nPATH=/stub:$PATH\n"
	cmd := exec.Command("docker", "run", "--rm", "--user", "0", "--entrypoint", "sh", image,
		"-c", caSetup+stub+block+"echo reached-after-block")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil {
		t.Errorf("block succeeded despite a failing update-ca-certificates:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Errorf("stderr lacks the failure output: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "reached-after-block") {
		t.Error("block did not stop on failure")
	}
}

// TestEntrypointUseraddQuiet: a macOS UID (501) and a high LDAP UID create the user and group with
// no useradd/groupadd range warning. Skipped without Docker or the image.
func TestEntrypointUseraddQuiet(t *testing.T) {
	image, block := entrypointBlock(t, `  group=$(getent group "$LODEV_GID"`, "-g \"$group\" \"$LODEV_USER\"\n  fi\n")
	for _, id := range []string{"501", "1500000"} {
		got, err := exec.Command("docker", "run", "--rm", "--user", "0", "--entrypoint", "sh",
			"-e", "LODEV_USER=kuroko", "-e", "LODEV_UID="+id, "-e", "LODEV_GID="+id, image,
			"-c", "set -e\nrm -rf /home/kuroko\n"+block+"id kuroko").CombinedOutput()
		if err != nil {
			t.Fatalf("uid %s: %v\n%s", id, err, got)
		}
		if strings.Contains(string(got), "warning") || strings.Contains(string(got), "useradd:") {
			t.Errorf("uid %s: warnings:\n%s", id, got)
		}
		if !strings.Contains(string(got), "uid="+id+"(kuroko)") {
			t.Errorf("uid %s: user not created:\n%s", id, got)
		}
	}
}

// homeRun runs the entrypoint's user block (useradd through the home build) as root in a local
// lodev/webserver image: the repo's skel replaces the image's /etc/skel, global and project (when
// non-nil) are the read-only homeadditional mounts, and /var/lib/lodev/home is a tmpfs owned by the
// user. It returns the script's report on the resulting home.
func homeRun(t *testing.T, env []string, global, project map[string]string) string {
	t.Helper()
	image, block := entrypointBlock(t, `  group=$(getent group "$LODEV_GID"`, "  # home: end\n")
	skel, err := filepath.Abs("webserver/skel")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"run", "--rm", "--user", "0", "--entrypoint", "sh",
		"-e", "LODEV_USER=kuroko", "-e", "LODEV_UID=501", "-e", "LODEV_GID=501",
		"-v", skel + ":/etc/skel:ro", "--tmpfs", "/var/lib/lodev/home:uid=501,gid=501"}
	for name, files := range map[string]map[string]string{"global": global, "project": project} {
		if files == nil {
			continue
		}
		d := t.TempDir()
		for f, c := range files {
			if err := os.WriteFile(filepath.Join(d, f), []byte(c), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		args = append(args, "-v", d+":/etc/lodev/homeadditional/"+name+":ro")
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	script := "set -e\n" + block + `cd /home/kuroko
echo "bashrc=$(grep -c '^HISTSIZE=10000$' .bashrc)"
echo "gitconfig=$(grep -c 'directory = \*' .gitconfig)"
echo "x=$(cat x 2>/dev/null || echo none)"
echo "psysh=$(readlink .config/psysh)"
echo "foreign=$(find /home/kuroko ! -user kuroko | tr '\n' ' ')"
if [ -e .my.cnf ]; then echo "mycnf-mode=$(stat -c '%U %a' .my.cnf)"; fi
echo "--- .my.cnf"
cat .my.cnf 2>/dev/null || echo none
`
	got, err := exec.Command("docker", append(args, image, "-c", script)...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	return string(got)
}

// TestEntrypointBuildsHome: the home is the skel, then the generated .my.cnf, then the global and
// project homeadditional folders (later wins); every file belongs to the user. Skipped without
// Docker or the image.
func TestEntrypointBuildsHome(t *testing.T) {
	// A .my.cnf in the project folder wins over the generated one; the project's x over the global's.
	got := homeRun(t, []string{"DB_HOST=mariadb", "DB_DATABASE=shop"},
		map[string]string{".gitconfig": "[user]\n\tname = global\n", "x": "global"},
		map[string]string{".my.cnf": "[client]\nuser=mine\n", "x": "project"})
	for _, want := range []string{"bashrc=1", "gitconfig=0", "x=project", "psysh=/var/lib/lodev/home/.config/psysh",
		"foreign=\n", "[client]\nuser=mine\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "host=mariadb") {
		t.Errorf("project .my.cnf must replace the generated one:\n%s", got)
	}

	// No homeadditional .my.cnf: the generated one, mode 600, owned by the user.
	got = homeRun(t, []string{"DB_HOST=mariadb", "DB_DATABASE=shop"}, map[string]string{"x": "global"}, map[string]string{})
	for _, want := range []string{"gitconfig=1", "x=global", "mycnf-mode=kuroko 600",
		"[client]\nuser=root\npassword=root\nhost=mariadb\ndefault-character-set=utf8mb4\n\n[mysql]\ndatabase=shop\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	// No database and no homeadditional folders at all: a [client]-only file. No DB_HOST: no file.
	got = homeRun(t, []string{"DB_HOST=mariadb"}, nil, nil)
	if !strings.Contains(got, "host=mariadb") || strings.Contains(got, "[mysql]") || !strings.Contains(got, "bashrc=1") {
		t.Errorf("want a [client]-only .my.cnf:\n%s", got)
	}
	if got = homeRun(t, nil, nil, nil); !strings.HasSuffix(got, "--- .my.cnf\nnone\n") {
		t.Errorf("no DB_HOST must leave no .my.cnf:\n%s", got)
	}
}
