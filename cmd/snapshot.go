package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/ui"
)

var snapshotNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type snapshotMeta struct {
	Created  time.Time `json:"created"`
	Services []string  `json:"services"`
	Lodev    string    `json:"lodev"`
}

// snapshotTargets are the project's own databases: its SQL database and its MongoDB one.
func snapshotTargets(p *config.Project) []dbTarget {
	var out []dbTarget
	if db := p.DB(); db != "" {
		out = append(out, dbTarget{db, p.DBName()})
	}
	if p.Uses("mongodb") {
		out = append(out, dbTarget{"mongodb", p.DBName()})
	}
	return out
}

func snapshotDir(project, name string) string {
	return filepath.Join(config.Home(), "snapshots", project, name)
}

func snapshotFile(service string) string {
	if service == "mongodb" {
		return "mongodb.archive.gz"
	}
	return service + ".sql.gz"
}

type snapshotEntry struct {
	name string
	meta snapshotMeta
	size int64
}

// snapshotsOf lists a project's snapshots, newest first.
func snapshotsOf(project string) ([]snapshotEntry, error) {
	dirs, err := os.ReadDir(filepath.Join(config.Home(), "snapshots", project))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var out []snapshotEntry
	for _, d := range dirs {
		dir := snapshotDir(project, d.Name())
		b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
		if err != nil {
			continue // not a snapshot (or a half one)
		}
		e := snapshotEntry{name: d.Name()}
		if json.Unmarshal(b, &e.meta) != nil {
			continue
		}
		for _, s := range e.meta.Services {
			if fi, err := os.Stat(filepath.Join(dir, snapshotFile(s))); err == nil {
				e.size += fi.Size()
			}
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b snapshotEntry) int { return b.meta.Created.Compare(a.meta.Created) })
	return out, nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func snapshotProject(c *cobra.Command) (*config.Project, error) {
	if name, _ := c.Flags().GetString("project"); name != "" {
		return projectArg([]string{name})
	}
	return currentProject()
}

func snapshotCreate(c *cobra.Command, args []string) error {
	p, err := snapshotProject(c)
	if err != nil {
		return err
	}
	name := time.Now().Format("20060102-150405")
	if len(args) > 0 {
		name = args[0]
	}
	if !snapshotNameRe.MatchString(name) {
		return fmt.Errorf("snapshot name %q: use A-Z a-z 0-9 . _ -", name)
	}
	targets := snapshotTargets(p)
	if len(targets) == 0 {
		return fmt.Errorf("project %s has no database to snapshot", p.Name)
	}
	for _, t := range targets {
		if !containerRunning("lodev-" + t.service) {
			return fmt.Errorf("lodev-%s is not running (lodev service enable %s)", t.service, t.service)
		}
	}
	dir := snapshotDir(p.Name, name)
	if _, err := os.Stat(dir); err == nil {
		if force, _ := c.Flags().GetBool("force"); !force {
			return fmt.Errorf("snapshot %s exists (--force replaces it)", name)
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	meta := snapshotMeta{Created: time.Now(), Lodev: Version}
	for _, t := range targets {
		if err := dumpFile(t, filepath.Join(dir, snapshotFile(t.service))); err != nil {
			os.RemoveAll(dir) // a half snapshot would look like a good one
			return fmt.Errorf("snapshot %s: %s: %w", name, t.service, err)
		}
		meta.Services = append(meta.Services, t.service)
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o644); err != nil {
		return err
	}
	ui.Success("Snapshot %s of %s: %s", name, p.Name, strings.Join(meta.Services, ", "))
	return nil
}

func dumpFile(t dbTarget, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := dumpTo(t, f, true); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func newSnapshotCmd() *cobra.Command {
	c := &cobra.Command{Use: "snapshot", Short: "Save and restore a project's databases"}
	c.PersistentFlags().StringP("project", "p", "", "a registered project instead of the one here")

	create := &cobra.Command{
		Use:   "create [name]",
		Short: "Dump the project's databases into a snapshot (default name: the time)",
		Args:  cobra.MaximumNArgs(1),
		RunE:  snapshotCreate,
	}
	create.Flags().Bool("force", false, "replace a snapshot of the same name")

	list := &cobra.Command{
		Use:   "list",
		Short: "List the project's snapshots, newest first",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := snapshotProject(c)
			if err != nil {
				return err
			}
			snaps, err := snapshotsOf(p.Name)
			if err != nil {
				return err
			}
			rows := make([][]string, len(snaps))
			for i, s := range snaps {
				rows[i] = []string{s.name, s.meta.Created.Format("2006-01-02 15:04"), humanSize(s.size), strings.Join(s.meta.Services, ", ")}
			}
			printTable(c.OutOrStdout(), []string{"NAME", "CREATED", "SIZE", "DATABASES"}, rows)
			return nil
		},
	}

	restore := &cobra.Command{
		Use:   "restore [name]",
		Short: "Load a snapshot over the project's databases (default: the newest)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := snapshotProject(c)
			if err != nil {
				return err
			}
			snaps, err := snapshotsOf(p.Name)
			if err != nil {
				return err
			}
			i := 0
			if len(args) > 0 {
				i = slices.IndexFunc(snaps, func(s snapshotEntry) bool { return s.name == args[0] })
			}
			if i < 0 || len(snaps) == 0 {
				name := "any"
				if len(args) > 0 {
					name = args[0]
				}
				return fmt.Errorf("no snapshot %s for %s", name, p.Name)
			}
			s := snaps[i]
			var over []string
			for _, svc := range s.meta.Services {
				over = append(over, p.DBName()+" ("+svc+")")
			}
			if yes, _ := c.Flags().GetBool("yes"); !yes && !confirmTyped(fmt.Sprintf("Restore %s over %s? Type 'yes':", s.name, strings.Join(over, ", ")), "yes") {
				return errors.New("aborted")
			}
			for _, svc := range s.meta.Services {
				t := dbTarget{svc, p.DBName()}
				f, err := os.Open(filepath.Join(snapshotDir(p.Name, s.name), snapshotFile(svc)))
				if err != nil {
					return err
				}
				err = restoreFrom(t, f)
				f.Close()
				if err != nil {
					return fmt.Errorf("restore %s: %s: %w", s.name, svc, err)
				}
			}
			ui.Success("Restored %s into %s", s.name, strings.Join(over, ", "))
			if noHooks, _ := c.Flags().GetBool("no-hooks"); noHooks {
				return nil
			}
			return runHooks(p, "post-snapshot-restore")
		},
	}
	restore.Flags().Bool("yes", false, "do not ask before overwriting the databases")
	restore.Flags().Bool("no-hooks", false, "do not run the post-snapshot-restore hooks")

	del := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := snapshotProject(c)
			if err != nil {
				return err
			}
			dir := snapshotDir(p.Name, args[0])
			if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil || !snapshotNameRe.MatchString(args[0]) {
				return fmt.Errorf("no snapshot %s for %s", args[0], p.Name)
			}
			return os.RemoveAll(dir)
		},
	}
	c.AddCommand(create, list, restore, del)
	return c
}

func init() { rootCmd.AddCommand(newSnapshotCmd()) }
