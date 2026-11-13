package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// healthPoll is how often waitHealthy asks Docker; swapped in tests.
var healthPoll = time.Second

// Swapped in tests (TestMain) so commands do not wait on real containers.
var (
	waitHealthyFn  = waitHealthy
	waitServicesFn = waitServicesHealthy
)

type healthState struct {
	Status   string
	ExitCode int
	Health   *struct {
		Status string
		Log    []struct{ Output string }
	}
	Restarts int `json:"-"`
}

func inspectState(name string) (healthState, error) {
	var s healthState
	out, err := dockerOutput("inspect", "--format", "{{.RestartCount}}|{{json .State}}", name)
	if err != nil {
		return s, err
	}
	n, js, _ := strings.Cut(out, "|")
	s.Restarts, _ = strconv.Atoi(n)
	return s, json.Unmarshal([]byte(js), &s)
}

// lastCheck is the newest healthcheck output, on one line.
func (s healthState) lastCheck() string {
	if s.Health == nil || len(s.Health.Log) == 0 {
		return ""
	}
	return strings.Join(strings.Fields(s.Health.Log[len(s.Health.Log)-1].Output), " ")
}

// waitHealthy blocks until container's healthcheck passes. It gives up at once when the container
// exited, keeps restarting or turned unhealthy, and after timeout otherwise; the error says where
// to look (logsCmd is the lodev command that shows its logs).
func waitHealthy(container string, timeout time.Duration, logsCmd string) error {
	deadline := time.Now().Add(timeout)
	for {
		s, err := inspectState(container)
		var reason string
		switch {
		case err != nil:
			// Not created yet, or Docker hiccuped: keep waiting until the deadline.
		case s.Status == "exited" || s.Status == "dead":
			reason = fmt.Sprintf("%s exited (code %d)", container, s.ExitCode)
		case s.Status == "restarting":
			reason = fmt.Sprintf("%s keeps restarting (%d restarts)", container, s.Restarts)
		case s.Health != nil && s.Health.Status == "healthy":
			return nil
		case s.Health != nil && s.Health.Status == "unhealthy":
			reason = fmt.Sprintf("%s is unhealthy: %s", container, s.lastCheck())
		}
		if reason == "" && time.Now().After(deadline) {
			reason = fmt.Sprintf("%s is not healthy after %s (container_timeout)", container, timeout)
			if c := s.lastCheck(); c != "" {
				reason += ": " + c
			}
		}
		if reason != "" {
			return fmt.Errorf("%s\nCheck:\n  %s\n  docker logs %s\n  docker inspect --format \"{{ json .State.Health }}\" %s",
				reason, logsCmd, container, container)
		}
		time.Sleep(healthPoll)
	}
}

// waitServicesHealthy waits for the shared services side by side and reports every one that failed.
// traefik gets its own line and the other services one line together; a failed line names the
// containers that failed.
func waitServicesHealthy(services []string, timeout time.Duration) error {
	type group struct {
		label, ok string
		services  []string
	}
	var groups []group
	shared := slices.DeleteFunc(slices.Clone(services), func(s string) bool { return s == "traefik" })
	if len(shared) < len(services) {
		groups = append(groups, group{"waiting lodev-traefik healthy", "lodev-traefik healthy", []string{"traefik"}})
	}
	if len(shared) > 0 {
		groups = append(groups, group{"waiting shared services healthy", "shared services healthy", shared})
	}
	labels := make([]string, len(groups))
	for i, g := range groups {
		labels[i] = g.label
	}
	l := ui.Spin(labels...)
	errs := make([]error, len(groups))
	var all sync.WaitGroup
	for gi, g := range groups {
		all.Go(func() {
			gerrs := make([]error, len(g.services))
			var wg sync.WaitGroup
			for i, svc := range g.services {
				wg.Go(func() { gerrs[i] = waitHealthy("lodev-"+svc, timeout, "lodev service logs "+svc) })
			}
			wg.Wait()
			var failed []string
			for i, err := range gerrs {
				if err != nil {
					failed = append(failed, "lodev-"+g.services[i])
				}
			}
			if errs[gi] = errors.Join(gerrs...); len(failed) > 0 {
				l.Fail(gi, strings.Join(failed, ", ")+" not healthy")
			} else {
				l.Done(gi, g.ok)
			}
		})
	}
	all.Wait()
	l.Stop()
	return errors.Join(errs...)
}

// waitWeb waits for the project's web container (its lodev-healthcheck).
func waitWeb(p *config.Project, g *config.Global) error {
	c := "lodev-" + p.Name + "-web"
	return waitShown(c, func() error { return waitHealthyFn(c, g.ContainerWait(), "lodev logs -s web") })
}

// waitDB waits for the shared database a project uses.
func waitDB(db string, g *config.Global) error {
	c := "lodev-" + db
	return waitShown(c, func() error { return waitHealthyFn(c, g.ContainerWait(), "lodev service logs "+db) })
}

// waitShown runs one health wait behind a spinner line for container.
func waitShown(container string, wait func() error) error {
	l := ui.Spin("waiting " + container + " healthy")
	err := wait()
	finishLine(l, 0, container, err)
	l.Stop()
	return err
}

func finishLine(l *ui.Lines, i int, container string, err error) {
	if err != nil {
		l.Fail(i, container+" not healthy")
	} else {
		l.Done(i, container+" healthy")
	}
}
