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

// TestWaitServicesShowsLines: traefik gets its own line, the shared services one line together;
// a failed line names the containers that failed.
func TestWaitServicesShowsLines(t *testing.T) {
	for _, tc := range []struct {
		name     string
		services []string
		exited   []string
		want     []string
		absent   []string
	}{
		{"all healthy", []string{"traefik", "mysql", "redis"}, nil,
			[]string{"waiting lodev-traefik healthy\n", "waiting shared services healthy\n", "✔ lodev-traefik healthy\n", "✔ shared services healthy\n"},
			[]string{"lodev-mysql", "lodev-redis"}},
		{"one shared fails", []string{"traefik", "mysql", "redis"}, []string{"lodev-mysql"},
			[]string{"✔ lodev-traefik healthy\n", "✗ lodev-mysql unhealthy\n"},
			[]string{"✔ shared services", "lodev-redis"}},
		{"several fail", []string{"traefik", "mysql", "redis", "opensearch"}, []string{"lodev-traefik", "lodev-mysql", "lodev-opensearch"},
			[]string{"✗ lodev-traefik unhealthy\n", "✗ lodev-mysql, lodev-opensearch unhealthy\n"},
			[]string{"✔"}},
		{"no traefik", []string{"mysql"}, nil,
			[]string{"waiting shared services healthy\n✔ shared services healthy\n"},
			[]string{"traefik"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, _, restore := ui.Capture(false)
			t.Cleanup(restore)
			oldOut, oldPoll := dockerOutput, healthPoll
			t.Cleanup(func() { dockerOutput, healthPoll = oldOut, oldPoll })
			healthPoll = time.Millisecond
			dockerOutput = func(a ...string) (string, error) {
				if slices.Contains(tc.exited, a[len(a)-1]) {
					return stExited, nil
				}
				return stHealthy, nil
			}
			err := waitServicesHealthy(tc.services, 20*time.Millisecond)
			if (err != nil) != (len(tc.exited) > 0) {
				t.Errorf("error %v", err)
			}
			for _, c := range tc.exited {
				if !strings.Contains(err.Error(), c+" exited") {
					t.Errorf("error lacks %s: %v", c, err)
				}
			}
			for _, w := range tc.want {
				if !strings.Contains(o.String(), w) {
					t.Errorf("missing %q in %q", w, o.String())
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(o.String(), a) {
					t.Errorf("unexpected %q in %q", a, o.String())
				}
			}
		})
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
	if got, want := o.String(), "waiting lodev-shop-web healthy\n✔ lodev-shop-web healthy\n"; got != want {
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
	if got := o.String(); !strings.HasSuffix(got, "✗ lodev-mariadb unhealthy\n") {
		t.Errorf("got %q", got)
	}
}
