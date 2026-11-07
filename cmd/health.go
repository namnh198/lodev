package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"lodev/internal/config"
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
func waitServicesHealthy(services []string, timeout time.Duration) error {
	errs := make([]error, len(services))
	var wg sync.WaitGroup
	for i, svc := range services {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = waitHealthy("lodev-"+svc, timeout, "lodev service logs "+svc)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// waitWeb waits for the project's web container (its lodev-healthcheck).
func waitWeb(p *config.Project, g *config.Global) error {
	return waitHealthyFn("lodev-"+p.Name+"-web", g.ContainerWait(), "lodev logs -s web")
}

// waitDB waits for the shared database a project uses.
func waitDB(db string, g *config.Global) error {
	return waitHealthyFn("lodev-"+db, g.ContainerWait(), "lodev service logs "+db)
}
