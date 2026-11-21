package containers

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// runHealthcheck runs webserver/healthcheck.sh with supervisorctl and curl stubbed to succeed.
func runHealthcheck(t *testing.T, env ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	bin, work := t.TempDir(), t.TempDir()
	for name, body := range map[string]string{"supervisorctl": "echo x RUNNING", "curl": "exit 0"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(work, ".lodev"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".lodev", "config.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "webserver/healthcheck.sh")
	cmd.Env = append([]string{"PATH=" + bin + ":" + os.Getenv("PATH"), "LODEV_WORKDIR=" + work, "LODEV_WEBSERVER=nginx"}, env...)
	out, err := cmd.Output()
	return strings.TrimSpace(strings.ReplaceAll(string(out), work, "/app")), err
}

// A nextjs container is healthy once the Next server listens, whatever php-fpm says: nginx
// answers 502 until then.
func TestHealthcheckNextjs(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := "LODEV_NEXTJS_PORT=" + strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	if out, err := runHealthcheck(t, port); err != nil || out != "/app:OK nginx:OK php-fpm:OK nextjs:OK" {
		t.Errorf("listening: %q %v", out, err)
	}
	l.Close()
	if out, err := runHealthcheck(t, port); err == nil || out != "/app:OK nginx:OK php-fpm:OK nextjs:FAILED" {
		t.Errorf("not listening: %q %v", out, err)
	}
	if out, err := runHealthcheck(t); err != nil || out != "/app:OK nginx:OK php-fpm:OK phpstatus:OK" {
		t.Errorf("php project: %q %v", out, err)
	}
}
