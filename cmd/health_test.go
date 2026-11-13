package cmd

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// stubInspect answers `docker inspect` for waitHealthy with the given states, one per call
// (the last one repeats).
func stubInspect(t *testing.T, states ...string) *int {
	t.Helper()
	oldOut, oldPoll := dockerOutput, healthPoll
	t.Cleanup(func() { dockerOutput, healthPoll = oldOut, oldPoll })
	healthPoll = time.Millisecond
	n := 0
	dockerOutput = func(a ...string) (string, error) {
		if a[0] != "inspect" {
			t.Fatalf("unexpected docker %q", a)
		}
		s := states[min(n, len(states)-1)]
		n++
		return s, nil
	}
	return &n
}

const (
	stStarting  = `0|{"Status":"running","Health":{"Status":"starting","Log":[]}}`
	stHealthy   = `0|{"Status":"running","Health":{"Status":"healthy","Log":[{"Output":"ok"}]}}`
	stUnhealthy = `0|{"Status":"running","Health":{"Status":"unhealthy","Log":[{"Output":"/var/www/html:OK nginx:OK php-fpm:FAILED phpstatus:FAILED \n"}]}}`
	stRestart   = `3|{"Status":"restarting","Health":{"Status":"starting","Log":[]}}`
	stExited    = `1|{"Status":"exited","ExitCode":1,"Health":{"Status":"unhealthy","Log":[]}}`
)

func TestWaitHealthyBecomesHealthy(t *testing.T) {
	n := stubInspect(t, stStarting, stStarting, stHealthy)
	if err := waitHealthy("lodev-shop-web", time.Second, "lodev logs -s web"); err != nil || *n != 3 {
		t.Fatalf("err %v after %d polls", err, *n)
	}
}

func TestWaitHealthyFailures(t *testing.T) {
	for _, c := range []struct{ state, want string }{
		{stUnhealthy, "lodev-shop-web is unhealthy: /var/www/html:OK nginx:OK php-fpm:FAILED phpstatus:FAILED"},
		{stRestart, "lodev-shop-web keeps restarting (3 restarts)"},
		{stExited, "lodev-shop-web exited (code 1)"},
		{stStarting, "lodev-shop-web is not healthy after 20ms"},
	} {
		stubInspect(t, c.state)
		err := waitHealthy("lodev-shop-web", 20*time.Millisecond, "lodev logs -s web")
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want prefix %q", c.state, err, c.want)
			continue
		}
		for _, hint := range []string{"Check:\n", "  lodev logs -s web\n", "  docker logs lodev-shop-web\n",
			`  docker inspect --format "{{ json .State.Health }}" lodev-shop-web`} {
			if !strings.Contains(err.Error(), hint) {
				t.Errorf("%s: missing %q in:\n%s", c.state, hint, err)
			}
		}
	}
}

func TestWaitAllHealthyListsEveryFailure(t *testing.T) {
	oldOut, oldPoll := dockerOutput, healthPoll
	t.Cleanup(func() { dockerOutput, healthPoll = oldOut, oldPoll })
	healthPoll = time.Millisecond
	dockerOutput = func(a ...string) (string, error) {
		if a[len(a)-1] == "lodev-redis" {
			return stHealthy, nil
		}
		return stExited, nil
	}
	err := waitServicesHealthy([]string{"mysql", "redis", "opensearch"}, 20*time.Millisecond)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"lodev-mysql exited", "lodev-opensearch exited", "lodev service logs mysql", "lodev service logs opensearch"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%s", want, err)
		}
	}
	if strings.Contains(err.Error(), "lodev-redis") {
		t.Errorf("healthy redis reported:\n%s", err)
	}
}

func TestWaitWebUsesContainerTimeout(t *testing.T) {
	old := waitHealthyFn
	t.Cleanup(func() { waitHealthyFn = old })
	var got []string
	waitHealthyFn = func(c string, d time.Duration, logs string) error {
		got = append(got, c+" "+d.String()+" "+logs)
		return nil
	}
	g, _ := config.ParseGlobal([]byte("container_timeout: 90s\n"))
	if err := waitWeb(&config.Project{Name: "shop"}, g); err != nil {
		t.Fatal(err)
	}
	if err := waitDB("mariadb", g); err != nil {
		t.Fatal(err)
	}
	want := []string{"lodev-shop-web 1m30s lodev logs -s web", "lodev-mariadb 1m30s lodev service logs mariadb"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWaitServicesShowsLines(t *testing.T) {
	o, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	oldOut, oldPoll := dockerOutput, healthPoll
	t.Cleanup(func() { dockerOutput, healthPoll = oldOut, oldPoll })
	healthPoll = time.Millisecond
	dockerOutput = func(a ...string) (string, error) {
		if a[len(a)-1] == "lodev-redis" {
			return stHealthy, nil
		}
		return stExited, nil
	}
	err := waitServicesHealthy([]string{"mysql", "redis"}, 20*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "lodev-mysql exited") {
		t.Errorf("error %v", err)
	}
	for _, want := range []string{"waiting lodev-mysql healthy\n", "waiting lodev-redis healthy\n", "✓ lodev-redis healthy\n", "✗ lodev-mysql not healthy\n"} {
		if !strings.Contains(o.String(), want) {
			t.Errorf("missing %q in %q", want, o.String())
		}
	}
}

func TestWaitWebShowsLine(t *testing.T) {
	o, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	old := waitHealthyFn
	t.Cleanup(func() { waitHealthyFn = old })
	waitHealthyFn = func(string, time.Duration, string) error { return nil }
	if err := waitWeb(&config.Project{Name: "shop"}, &config.Global{}); err != nil {
		t.Fatal(err)
	}
	if got, want := o.String(), "waiting lodev-shop-web healthy\n✓ lodev-shop-web healthy\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWaitDBShowsFailure(t *testing.T) {
	o, _, restore := ui.Capture(false)
	t.Cleanup(restore)
	old := waitHealthyFn
	t.Cleanup(func() { waitHealthyFn = old })
	boom := errors.New("lodev-mariadb exited (code 1)")
	waitHealthyFn = func(string, time.Duration, string) error { return boom }
	if err := waitDB("mariadb", &config.Global{}); err != boom {
		t.Fatalf("error %v", err)
	}
	if got := o.String(); !strings.HasSuffix(got, "✗ lodev-mariadb not healthy\n") {
		t.Errorf("got %q", got)
	}
}
