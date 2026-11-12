package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"lodev/assets"
	"lodev/internal/certs"
	"lodev/internal/config"
	"lodev/internal/docker"
	"lodev/internal/hosts"
	"lodev/internal/templates"
	"lodev/internal/ui"
)

// PortsToCheck lists host ports to preflight; ports our own running containers hold are skipped.
func PortsToCheck(g *config.Global, traefikRunning, mysqlRunning, mariadbRunning, mongodbRunning bool) []int {
	if traefikRunning {
		return nil
	}
	ports := []int{g.Ports.HTTP, g.Ports.HTTPS, g.Ports.Dashboard}
	for _, db := range []struct {
		port    string
		running bool
	}{{g.Ports.MySQL, mysqlRunning}, {g.Ports.MariaDB, mariadbRunning}, {g.Ports.MongoDB, mongodbRunning}} {
		if db.port != "" && !db.running {
			n, _ := strconv.Atoi(db.port)
			ports = append(ports, n)
		}
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

// ensureDBConfig writes the default ~/.lodev/services/<db>/lodev.cnf for mysql and mariadb when
// missing; the user's edits are kept (and moved from the older ~/.lodev/<db>/). Created here, not
// by Docker, which would make a root-owned folder.
func ensureDBConfig() error {
	data, err := assets.FS.ReadFile("templates/db.cnf")
	if err != nil {
		return err
	}
	for _, db := range []string{"mysql", "mariadb"} {
		f := filepath.Join(config.Home(), "services", db, "lodev.cnf")
		if _, err := os.Stat(f); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			return err
		}
		oldDir := filepath.Join(config.Home(), db)
		if err := os.Rename(filepath.Join(oldDir, "lodev.cnf"), f); err == nil {
			os.Remove(oldDir) // only when now empty
			continue
		}
		if err := os.WriteFile(f, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// portCheck is checkPort for lodev doctor; swapped in tests.
var portCheck = checkPort

func servicesUp() error {
	if err := docker.Ping(); err != nil {
		return err
	}
	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	for _, p := range PortsToCheck(g, docker.Running("lodev-traefik"), docker.Running("lodev-mysql"), docker.Running("lodev-mariadb"), docker.Running("lodev-mongodb")) {
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
	for _, old := range []string{"compose.yaml", "compose-full.yaml"} { // names before services.compose-*.yaml
		os.Remove(filepath.Join(filepath.Dir(servicesFile()), old))
	}
	if err := mergeServicesCompose(); err != nil {
		return err
	}
	if err := certs.Ensure("lodev-services", config.ReservedDomains()); err != nil {
		return err
	}
	if err := certs.SyncTraefik(); err != nil {
		return err
	}
	if err := hosts.Apply("lodev-services", config.ReservedDomains()); err != nil {
		return err
	}
	if err := ensureDBConfig(); err != nil {
		return err
	}
	warnImageChange(g)
	if err := prepareServiceData(g, g.EnabledServices()); err != nil {
		return err
	}
	if err := servicesCompose(append([]string{"up", "-d", "traefik"}, g.EnabledServices()...)...); err != nil {
		return err
	}
	return waitServicesFn(append([]string{"traefik"}, g.EnabledServices()...), g.ContainerWait())
}

// printServiceURLs lists the shared services' URLs; printed once, not per project start.
func printServiceURLs() error {
	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	fmt.Println()
	ui.Success("Traefik dashboard: http://localhost:%d", g.Ports.Dashboard)
	if g.Services["phpmyadmin"] {
		ui.Success("phpMyAdmin:        https://phpmyadmin.%s", g.TLD)
	}
	if g.Services["mailpit"] {
		ui.Success("Mailpit:           https://mailpit.%s", g.TLD)
	}
	if g.Services["rabbitmq"] {
		ui.Success("RabbitMQ:          https://rabbitmq.%s", g.TLD)
	}
	if g.Services["mongo-express"] {
		ui.Success("Mongo Express:     https://mongo.%s", g.TLD)
	}
	return nil
}

const servicesExample = `# Rename to <name>.compose.yml to merge it into the shared services; run lodev service start after editing.
# services:
#   opensearch:
#     mem_limit: 3g
#   adminer:
#     image: adminer
#     networks: [lodev]
`

// mergeServicesCompose merges ~/.lodev/services/*.compose.{yml,yaml} over the generated services.compose-base.yaml.
func mergeServicesCompose() error {
	dir := filepath.Dir(servicesFile())
	example := filepath.Join(dir, "example.compose.yml.example")
	if _, err := os.Stat(example); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(example, []byte(servicesExample), 0o644); err != nil {
			return err
		}
	}
	extra, err := extraComposeFiles(dir)
	if err != nil {
		return err
	}
	return mergeComposeFiles("lodev-services", servicesFile(), servicesFullFile(), extra)
}

// warnImageChange warns before a database container is recreated on another image (e.g. a version change).
func warnImageChange(g *config.Global) {
	for _, svc := range []string{"mysql", "mariadb", "mongodb"} {
		cur, err := dockerOutput("inspect", "-f", "{{.Config.Image}}", "lodev-"+svc)
		if err != nil { // no container yet
			continue
		}
		if want := g.Image(svc); cur != want {
			ui.Warn("lodev-%s: %s → %s; a major downgrade may not read the existing data, dump it first", svc, cur, want)
		}
	}
}

// prepareServiceData creates the data folders of svcs (with mount_volume) and, into an empty folder,
// copies the service's old named volume once; the volume is kept.
func prepareServiceData(g *config.Global, svcs []string) error {
	for _, svc := range svcs {
		path := g.MountPath(svc)
		if path == "" {
			continue
		}
		_, statErr := os.Stat(path)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
		if svc == "opensearch" && errors.Is(statErr, fs.ErrNotExist) {
			// It runs as UID 1000 and never chowns its data dir (the others do). Only a folder lodev
			// just made: chmod fails on one opensearch already owns.
			if err := os.Chmod(path, 0o777); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(path)
		if errors.Is(err, fs.ErrPermission) {
			continue // owned by the service's UID since its first start (Linux): it has data
		}
		if err != nil {
			return err
		}
		vol := "lodev-" + svc
		if len(entries) > 0 {
			continue
		}
		// Inspect first: `docker run -v` would create a missing volume.
		if _, err := dockerOutput("volume", "inspect", vol); err != nil {
			continue
		}
		// The container is named like the volume; never copy files a running database is writing.
		if id, _ := dockerOutput("ps", "-aq", "--filter", "name=^"+vol+"$"); id != "" {
			if _, err := dockerOutput("stop", vol); err != nil {
				return err
			}
		}
		if err := dockerRun("run", "--rm", "--user", "0", "--entrypoint", "cp",
			"-v", vol+":/from", "-v", path+":/to", g.Image(svc), "-a", "/from/.", "/to/"); err != nil {
			return fmt.Errorf("copy volume %s to %s: %w (empty %s before retrying)", vol, path, err, path)
		}
		ui.Info("Copied volume %s to %s (the volume is kept)", vol, path)
	}
	return nil
}

// checkDisable refuses to disable mongodb while mongo-express is on: compose would start it again (depends_on).
// checkDisable refuses to turn off a service that one staying on needs.
func checkDisable(svcs []string, g *config.Global) error {
	if slices.Contains(svcs, "mongodb") && g.Services["mongo-express"] && !slices.Contains(svcs, "mongo-express") {
		return errors.New("mongo-express needs mongodb: run `lodev service disable mongo-express` first")
	}
	return nil
}

const volumesWarning = "This deletes all shared service data: MySQL, MariaDB, Redis, OpenSearch, RabbitMQ and MongoDB."

// startService starts one shared service from the existing compose file.
func startService(svc string) error {
	g, err := config.LoadGlobal()
	if err != nil {
		return err
	}
	if err := prepareServiceData(g, []string{svc}); err != nil {
		return err
	}
	return servicesCompose("up", "-d", svc)
}

func runningProjects() ([]string, error) {
	out, err := dockerOutput("ps", "--filter", "name=^lodev-.*-web$", "--format", "{{.Names}}")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Fields(out), nil
}

// poweroff takes every running project down with its own compose file, then the shared services.
func poweroff() error {
	ui.Info("Powering off lodev: stopping all projects and shared services")
	running, err := runningProjects()
	if err != nil {
		return err
	}
	for _, c := range running {
		name := strings.TrimSuffix(strings.TrimPrefix(c, "lodev-"), "-web")
		ui.Info("Stopping project %s", name)
		// compose labels each container with the folder of its compose file: <root>/.lodev.
		dir, err := dockerOutput("inspect", "-f", `{{index .Config.Labels "com.docker.compose.project.working_dir"}}`, c)
		if err != nil {
			return err
		}
		if p, err := config.LoadProject(filepath.Dir(dir)); err == nil {
			// down, not stop: service stop below removes the lodev network these containers use.
			if err := projectCompose(p, "down"); err != nil {
				return err
			}
			continue
		}
		// The project folder is gone, and its compose file with it: remove by compose's label.
		ids, err := dockerOutput("ps", "-aq", "--filter", "label=com.docker.compose.project=lodev-"+name)
		if err != nil {
			return err
		}
		if err := dockerRun(append([]string{"rm", "-f"}, strings.Fields(ids)...)...); err != nil {
			return err
		}
	}
	if servicesComposeFile() != "" {
		if err := servicesCompose("down"); err != nil {
			return err
		}
	}
	ui.Success("lodev is powered off")
	return nil
}

// sharedServiceArgs checks every name before anything changes; repeats are dropped.
func sharedServiceArgs(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		if !slices.Contains(config.SharedServices, a) {
			return nil, fmt.Errorf("unknown service %q (allowed: %s)", a, strings.Join(config.SharedServices, ", "))
		}
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out, nil
}

// setServices turns svcs on or off in one config write.
func setServices(svcs []string, on bool) error {
	old, err := loadGlobalForEdit()
	if err != nil {
		return err
	}
	changes := make([]change, len(svcs))
	for i, s := range svcs {
		changes[i] = change{key: []string{"services", s}, value: on}
	}
	_, _, err = applyGlobalConfig(old, changes)
	return err
}

func init() {
	servicesCmd := &cobra.Command{Use: "service", Aliases: []string{"services", "svc"}, Short: "Manage the shared services stack"}

	up := &cobra.Command{
		Use:     "start",
		Aliases: []string{"up"},
		Short:   "Start Traefik and all enabled shared services",
		Args:    cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := withLock(servicesUp); err != nil {
				return err
			}
			return printServiceURLs()
		},
	}

	var volumes, yes bool
	down := &cobra.Command{
		Use:     "stop",
		Aliases: []string{"down"},
		Short:   "Stop the shared services (data volumes are kept unless --volumes)",
		Args:    cobra.NoArgs,
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
				if !yes && !confirmTyped(volumesWarning+" Type 'yes' to continue:", "yes") {
					return errors.New("aborted")
				}
				sub = append(sub, "-v")
			}
			return servicesCompose(sub...)
		},
	}
	down.Flags().BoolVar(&volumes, "volumes", false, "also delete all shared service data (MySQL, MariaDB, Redis, OpenSearch, RabbitMQ, MongoDB)")
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
		Use:   "enable <service>...",
		Short: "Enable shared services and start them",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svcs, err := sharedServiceArgs(args)
			if err != nil {
				return err
			}
			return withLock(func() error {
				if err := setServices(svcs, true); err != nil {
					return err
				}
				return servicesUp()
			})
		},
	}

	disable := &cobra.Command{
		Use:   "disable <service>...",
		Short: "Disable shared services and remove their containers (data is kept)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svcs, err := sharedServiceArgs(args)
			if err != nil {
				return err
			}
			g, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			if err := checkDisable(svcs, g); err != nil {
				return err
			}
			if err := withLock(func() error { return setServices(svcs, false) }); err != nil {
				return err
			}
			if err := servicesCompose(append([]string{"rm", "-sf"}, svcs...)...); err != nil {
				return err
			}
			// phpMyAdmin's server list (PMA_HOSTS) follows the enabled databases.
			if slices.Contains(svcs, "mysql") || slices.Contains(svcs, "mariadb") {
				if err := withLock(servicesUp); err != nil {
					return err
				}
			}
			for _, svc := range svcs {
				if err := warnProjectsUsing(svc); err != nil {
					return err
				}
			}
			return nil
		},
	}

	servicesCmd.AddCommand(up, down, status, logs, enable, disable)
	rootCmd.AddCommand(servicesCmd, &cobra.Command{
		Use:   "poweroff",
		Short: "Stop all projects and the shared services",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return withLock(poweroff) },
	})
}
