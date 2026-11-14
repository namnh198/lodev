package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/dockertools"
	"lodev/internal/hosts"
	"lodev/internal/registry"
)

const (
	levelOK = iota
	levelWarn
	levelFail
)

type checkResult struct {
	Name, Detail, Fix string
	Level             int
}

// toolInstalled is the version of a pinned docker tool lodev has; swapped in tests.
var toolInstalled = dockertools.Installed

// containerUp reports whether name runs, asking through dockerOutput like the healthchecks.
func containerUp(name string) bool {
	s, err := inspectState(name)
	return err == nil && s.Status == "running"
}

// bytesOf reads docker's sizes ("4.5GB", "500MB (12%)") as bytes; 0 when it cannot.
func bytesOf(s string) float64 {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0
	}
	v := f[0]
	for _, u := range []struct {
		suffix string
		mul    float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1}} {
		if n, ok := strings.CutSuffix(v, u.suffix); ok {
			x, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return 0
			}
			return x * u.mul
		}
	}
	return 0
}

// runDoctor checks the setup; the bool is whether any check failed.
func runDoctor() ([]checkResult, bool) {
	var rs []checkResult
	add := func(name string, level int, detail, fix string) {
		rs = append(rs, checkResult{Name: name, Detail: detail, Fix: fix, Level: level})
	}
	dockerOK := dockerPing() == nil
	if dockerOK {
		add("Docker", levelOK, "running", "")
	} else {
		add("Docker", levelFail, "not reachable", "start Docker Desktop (or the docker service)")
	}

	g, err := config.LoadGlobal()
	if err != nil {
		add("Global config", levelFail, err.Error(), "fix ~/.lodev/config.yaml or run lodev config-global")
		g, _ = config.ParseGlobal(nil)
	} else {
		add("Global config", levelOK, "valid", "")
	}

	for _, tool := range []struct {
		name   string
		t      dockertools.Tool
		system bool
	}{{"Compose", dockertools.Compose, g.UseDockerComposeSystem}, {"Buildx", dockertools.Buildx, g.UseDockerBuildxSystem}} {
		switch {
		case tool.system || runtime.GOOS == "windows":
			add(tool.name, levelOK, "the system's", "")
		case toolInstalled(tool.t) == tool.t.Version():
			add(tool.name, levelOK, tool.t.Version(), "")
		default:
			add(tool.name, levelFail, tool.t.Name()+" "+tool.t.Version()+" not downloaded", "lodev downloads it on the next command that needs it")
		}
	}

	var portProblems []string
	for _, p := range PortsToCheck(g, dockerOK && containerUp("lodev-traefik"), dockerOK && containerUp("lodev-mysql"),
		dockerOK && containerUp("lodev-mariadb"), dockerOK && containerUp("lodev-mongodb")) {
		if err := portCheck(p); err != nil {
			portProblems = append(portProblems, strings.SplitN(err.Error(), "\n", 2)[0])
		}
	}
	if len(portProblems) > 0 {
		add("Ports", levelFail, strings.Join(portProblems, "; "), "stop what holds it, or lodev config-global --http-port/--https-port/--dashboard-port")
	} else {
		add("Ports", levelOK, "free or lodev's", "")
	}

	ca := filepath.Join(config.Home(), "rootCA", "rootCA.pem")
	switch _, err := os.Stat(ca); {
	case hasMkcert():
		add("Local CA", levelOK, "mkcert's", "")
	case err == nil:
		add("Local CA", levelOK, ca, "")
	default:
		add("Local CA", levelWarn, "no "+ca, "lodev service start creates it; trust it with the command in the README")
	}

	reg, _ := registry.Load()
	if b, err := os.ReadFile(hosts.File); err != nil {
		add("/etc/hosts", levelFail, err.Error(), "")
	} else {
		have := hosts.Domains(string(b))
		var missing []string
		for _, d := range append(config.ReservedDomains(), reg.AllDomains()...) {
			if !have[d] && !slices.Contains(missing, d) {
				missing = append(missing, d)
			}
		}
		if len(missing) > 0 {
			add("/etc/hosts", levelFail, "missing: "+strings.Join(missing, ", "), "lodev restart <project> / lodev service start")
		} else {
			add("/etc/hosts", levelOK, "all domains", "")
		}
	}

	if dockerOK {
		for _, svc := range append([]string{"traefik"}, g.EnabledServices()...) {
			s, err := inspectState("lodev-" + svc)
			switch {
			case err != nil || s.Status != "running":
				add("service "+svc, levelFail, "not running", "lodev service start")
			case s.Health != nil && s.Health.Status == "unhealthy":
				add("service "+svc, levelFail, s.lastCheck(), "lodev service logs "+svc)
			default:
				add("service "+svc, levelOK, "running", "")
			}
		}
		const traefikRow = "Traefik reads routes from files (no Docker socket)"
		if oldTraefik() {
			add(traefikRow, levelFail, "lodev-traefik still reads the Docker socket", "lodev service start")
		} else {
			add(traefikRow, levelOK, "yes", "")
		}
	}

	for _, name := range reg.Names() {
		p, err := config.LoadProject(reg[name].Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			add("project "+name, levelWarn, "folder or .lodev/config.yaml is gone", "lodev remove "+name)
			continue
		case err != nil:
			add("project "+name, levelFail, err.Error(), "fix .lodev/config.yaml")
			continue
		}
		if dockerOK {
			if s, err := inspectState("lodev-" + p.Name + "-web"); err == nil && s.Health != nil && s.Health.Status == "unhealthy" {
				add("project "+name, levelFail, s.lastCheck(), "lodev logs -s web (in the project)")
				continue
			}
		}
		add("project "+name, levelOK, "valid", "")
	}

	if dockerOK {
		if info, _ := dockerOutput("info", "--format", "{{.SecurityOptions}}"); strings.Contains(info, "name=userns") {
			add("userns-remap", levelWarn, "Docker runs with userns-remap", "lodev handles it (userns_mode: host); nothing to do")
		} else {
			add("userns-remap", levelOK, "off", "")
		}
		df, _ := dockerOutput("system", "df", "--format", "{{.Type}}\t{{.Reclaimable}}")
		var total float64
		for _, l := range strings.Split(df, "\n") {
			if _, v, ok := strings.Cut(l, "\t"); ok {
				total += bytesOf(v)
			}
		}
		if total > 5e9 {
			add("Disk", levelWarn, fmt.Sprintf("%.1f GB reclaimable in Docker", total/1e9), "lodev clean")
		} else {
			add("Disk", levelOK, fmt.Sprintf("%.1f GB reclaimable", total/1e9), "")
		}
	}

	failed := slices.ContainsFunc(rs, func(r checkResult) bool { return r.Level == levelFail })
	return rs, failed
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check Docker, ports, hosts, certificates, services and projects, and say how to fix problems",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			rs, failed := runDoctor()
			rows := make([][]string, len(rs))
			for i, r := range rs {
				rows[i] = []string{r.Name, [...]string{"✔", "!", "✘"}[r.Level], r.Detail, r.Fix}
			}
			printTable(c.OutOrStdout(), []string{"CHECK", "STATUS", "DETAIL", "FIX"}, rows)
			if failed {
				return errors.New("doctor found problems")
			}
			return nil
		},
	}
}

func init() { rootCmd.AddCommand(newDoctorCmd()) }
