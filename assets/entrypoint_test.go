package assets

import (
	"os/exec"
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
