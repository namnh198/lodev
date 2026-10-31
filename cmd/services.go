package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"lodev/internal/certs"
	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/hosts"
	"lodev/internal/registry"
	"lodev/internal/templates"
)

// PortsToCheck lists host ports to preflight; a running stack already owns them.
func PortsToCheck(g *config.Global, stackRunning bool) []int {
	if stackRunning {
		return nil
	}
	ports := []int{g.Ports.HTTP, g.Ports.HTTPS, g.Ports.Dashboard}
	if g.Ports.MySQL != "" {
		n, _ := strconv.Atoi(g.Ports.MySQL)
		ports = append(ports, n)
	}
	return ports
}

// checkPort dials instead of binding: macOS refuses 127.0.0.1:<1024 binds to
// normal users, while a successful dial reliably means something is listening.
func checkPort(p int) error {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(p), 300*time.Millisecond)
	if err != nil {
		return nil
	}
	c.Close()
	owner, _ := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(p), "-sTCP:LISTEN").Output()
	return fmt.Errorf("port %d is in use:\n%s", p, owner)
}

func servicesUp() error {
	if err := docker.Ping(); err != nil {
		return err
	}
	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	for _, p := range PortsToCheck(g, docker.Running("lodev-traefik")) {
		if err := checkPort(p); err != nil {
			return err
		}
	}
	data, err := templates.SharedCompose(g, config.Home())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(servicesFile()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(servicesFile(), data, 0o644); err != nil {
		return err
	}
	if err := certs.Ensure("lodev-services", config.ReservedDomains); err != nil {
		return err
	}
	if err := certs.SyncTraefik(); err != nil {
		return err
	}
	if err := hosts.Apply("lodev-services", config.ReservedDomains); err != nil {
		return err
	}
	if err := servicesCompose(append([]string{"up", "-d", "traefik"}, g.EnabledServices()...)...); err != nil {
		return err
	}
	fmt.Printf("\nTraefik dashboard: http://localhost:%d\n", g.Ports.Dashboard)
	if g.Services["phpmyadmin"] {
		fmt.Println("phpMyAdmin:        https://phpmyadmin.test")
	}
	if g.Services["mailpit"] {
		fmt.Println("Mailpit:           https://mailpit.test")
	}
	return nil
}

// startService starts one shared service from the existing compose file.
func startService(svc string) error {
	return servicesCompose("up", "-d", svc)
}

func runningProjects() ([]string, error) {
	out, err := docker.Output("ps", "--filter", "name=^lodev-.*-web$", "--format", "{{.Names}}")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Fields(out), nil
}

func sharedServiceArg(args []string) (string, error) {
	if !slices.Contains(config.SharedServices, args[0]) {
		return "", fmt.Errorf("unknown service %q (allowed: %s)", args[0], strings.Join(config.SharedServices, ", "))
	}
	return args[0], nil
}

func setService(svc string, on bool) error {
	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	g.Services[svc] = on
	return config.SaveGlobal(g)
}

func init() {
	servicesCmd := &cobra.Command{Use: "services", Short: "Manage the shared services stack"}

	up := &cobra.Command{
		Use:   "up",
		Short: "Start Traefik and all enabled shared services",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return servicesUp() },
	}

	var volumes, yes bool
	down := &cobra.Command{
		Use:   "down",
		Short: "Stop the shared services (data volumes are kept unless --volumes)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			running, err := runningProjects()
			if err != nil {
				return err
			}
			if len(running) > 0 {
				return fmt.Errorf("stop these projects first: %s", strings.Join(running, ", "))
			}
			sub := []string{"down"}
			if volumes {
				if !yes && !confirmTyped("This deletes all MySQL and OpenSearch data. Type 'yes' to continue:", "yes") {
					return errors.New("aborted")
				}
				sub = append(sub, "-v")
			}
			return servicesCompose(sub...)
		},
	}
	down.Flags().BoolVar(&volumes, "volumes", false, "also delete MySQL and OpenSearch data")
	down.Flags().BoolVar(&yes, "yes", false, "skip confirmation")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show shared service containers",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return servicesCompose("ps") },
	}

	logs := &cobra.Command{
		Use:   "logs [service]",
		Short: "Follow logs of the shared services",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return servicesCompose(append([]string{"logs", "-f"}, args...)...)
		},
	}

	enable := &cobra.Command{
		Use:   "enable <service>",
		Short: "Enable a shared service and start it",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := sharedServiceArg(args)
			if err != nil {
				return err
			}
			if err := setService(svc, true); err != nil {
				return err
			}
			return servicesUp()
		},
	}

	disable := &cobra.Command{
		Use:   "disable <service>",
		Short: "Disable a shared service and remove its container (data is kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := sharedServiceArg(args)
			if err != nil {
				return err
			}
			if err := setService(svc, false); err != nil {
				return err
			}
			if err := servicesCompose("rm", "-sf", svc); err != nil {
				return err
			}
			reg, err := registry.Load()
			if err != nil {
				return err
			}
			for _, name := range reg.Names() {
				if p, err := config.LoadProject(reg[name].Path); err == nil && p.Uses(svc) {
					fmt.Printf("warning: project %s uses %s\n", name, svc)
				}
			}
			return nil
		},
	}

	servicesCmd.AddCommand(up, down, status, logs, enable, disable)
	rootCmd.AddCommand(servicesCmd)
}
