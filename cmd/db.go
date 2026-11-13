package cmd

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var dbServices = []string{"mysql", "mariadb", "mongodb"}

// dbNameRe is what lodev puts between backticks in SQL (and in MongoDB names) without escaping.
var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_$-]+$`)

// mongoURI is the shared MongoDB's root login (MONGO_INITDB_ROOT_* in the services compose).
const mongoURI = "--uri=mongodb://root:root@localhost:27017/?authSource=admin"

type dbTarget struct{ service, db string }

// dbTargetFor picks the service (-s, else the project's database) and the database (the
// argument, else the project's).
func dbTargetFor(args []string, service string) (dbTarget, error) {
	p, _ := currentProject() // nil outside a project
	if service == "" {
		switch {
		case p != nil && p.DB() != "":
			service = p.DB()
		case p != nil && p.Uses("mongodb"):
			service = "mongodb"
		default:
			service = "mysql"
		}
	}
	if !slices.Contains(dbServices, service) {
		return dbTarget{}, fmt.Errorf("--service %q: use one of %s", service, strings.Join(dbServices, ", "))
	}
	t := dbTarget{service: service}
	switch {
	case len(args) > 0:
		if !dbNameRe.MatchString(args[0]) {
			return dbTarget{}, fmt.Errorf("database name %q: use A-Z a-z 0-9 _ $ -", args[0])
		}
		t.db = args[0]
	case p != nil:
		t.db = p.DBName()
	default:
		return dbTarget{}, errors.New("give the database name (not in a lodev project)")
	}
	return t, nil
}

// dbExecArgs runs cmd in the service container with stdin attached; the SQL password goes in
// the environment so the client prints no "password on the command line" warning into a dump.
func dbExecArgs(t dbTarget, cmd ...string) []string {
	args := []string{"exec", "-i"}
	if t.service != "mongodb" {
		args = append(args, "-e", "MYSQL_PWD=root")
	}
	return append(append(args, "lodev-"+t.service), cmd...)
}

func dbDumpArgs(t dbTarget) []string {
	switch t.service {
	case "mongodb":
		return dbExecArgs(t, "mongodump", mongoURI, "--db="+t.db, "--archive")
	case "mariadb":
		return dbExecArgs(t, "mariadb-dump", "-uroot", "--single-transaction", "--routines", "--triggers", "--events", "--no-tablespaces", t.db)
	}
	return dbExecArgs(t, "mysqldump", "-uroot", "--single-transaction", "--routines", "--triggers", "--events", "--no-tablespaces", t.db)
}

func dbRestoreArgs(t dbTarget) []string {
	switch t.service {
	case "mongodb":
		// Any source database lands in t.db; --drop replaces its collections, like a SQL dump's DROP TABLE.
		return dbExecArgs(t, "mongorestore", mongoURI, "--archive", "--drop", "--nsFrom=$db$.$col$", "--nsTo="+t.db+".$col$")
	case "mariadb":
		return dbExecArgs(t, "mariadb", "-uroot", t.db) // MariaDB 11 has no mysql client
	}
	return dbExecArgs(t, "mysql", "-uroot", t.db)
}

func dbDefaultFile(t dbTarget, gz bool, now time.Time) string {
	ext := ".sql"
	if t.service == "mongodb" {
		ext = ".archive"
	}
	if gz {
		ext += ".gz"
	}
	return t.db + "-" + now.Format("20060102-150405") + ext
}

// dumpTo streams t's dump to w, gzipped when gz. Shared by db-export and snapshot.
func dumpTo(t dbTarget, w io.Writer, gz bool) error {
	if !gz {
		return dockerStream(dbDumpArgs(t), nil, w)
	}
	zw := gzip.NewWriter(w)
	if err := dockerStream(dbDumpArgs(t), nil, zw); err != nil {
		return err
	}
	return zw.Close()
}

// restoreFrom loads a plain or gzipped dump into t, creating the SQL database first. Shared by
// db-import and snapshot.
func restoreFrom(t dbTarget, r io.Reader) error {
	br := bufio.NewReader(r)
	r = br
	if magic, _ := br.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return err
		}
		defer zr.Close()
		r = zr
	}
	if t.service != "mongodb" {
		if err := dbExec(t.service, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", t.db)); err != nil {
			return err
		}
	}
	return dockerStream(dbRestoreArgs(t), r, nil)
}

// dbFlags reads -s and -c and resolves the target, checking its container runs.
func dbFlags(c *cobra.Command, args []string) (dbTarget, bool, error) {
	service, _ := c.Flags().GetString("service")
	compress, _ := c.Flags().GetString("compress")
	if compress != "" && compress != "gz" {
		return dbTarget{}, false, fmt.Errorf("--compress %q: only gz is supported", compress)
	}
	t, err := dbTargetFor(args, service)
	if err != nil {
		return dbTarget{}, false, err
	}
	if !containerRunning("lodev-" + t.service) {
		return dbTarget{}, false, fmt.Errorf("lodev-%s is not running (lodev service enable %s)", t.service, t.service)
	}
	return t, compress == "gz", nil
}

func dbCmd(use, short string, run func(c *cobra.Command, t dbTarget, gz bool, file string) error) *cobra.Command {
	c := &cobra.Command{
		Use:   use + " [database]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			t, gz, err := dbFlags(c, args)
			if err != nil {
				return err
			}
			file, _ := c.Flags().GetString("file")
			return run(c, t, gz, file)
		},
	}
	c.Flags().StringP("service", "s", "", "mysql | mariadb | mongodb (default: the project's database)")
	c.Flags().StringP("compress", "c", "", "gz: gzip the dump (import detects gzip by itself)")
	c.Flags().StringP("file", "f", "", "dump file; - for stdout/stdin")
	return c
}

func newDBExportCmd() *cobra.Command {
	return dbCmd("db-export", "Dump a database of the shared mysql, mariadb or mongodb (default: the project's)",
		func(c *cobra.Command, t dbTarget, gz bool, file string) error {
			if file == "" {
				file = dbDefaultFile(t, gz, time.Now())
			}
			var w io.Writer = c.OutOrStdout()
			var f *os.File
			if file != "-" {
				var err error
				if f, err = os.Create(file); err != nil {
					return err
				}
				defer f.Close()
				w = f
			}
			err := dumpTo(t, w, gz)
			if err == nil && f != nil {
				err = f.Close()
			}
			if err != nil {
				if f != nil {
					os.Remove(file) // a half dump would look like a good one
				}
				return fmt.Errorf("dump %s from %s: %w", t.db, t.service, err)
			}
			if f != nil {
				fmt.Fprintf(c.ErrOrStderr(), "Exported %s (%s) to %s\n", t.db, t.service, file)
			}
			return nil
		})
}

func newDBImportCmd() *cobra.Command {
	c := dbCmd("db-import", "Load a dump (plain or gzip) into a database of the shared mysql, mariadb or mongodb",
		func(c *cobra.Command, t dbTarget, _ bool, file string) error {
			var r io.Reader
			switch file {
			case "":
				return errors.New("give the dump with -f <file> (or -f - for stdin)")
			case "-":
				r = c.InOrStdin()
			default:
				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()
				r = f
			}
			if err := restoreFrom(t, r); err != nil {
				return fmt.Errorf("import into %s on %s: %w", t.db, t.service, err)
			}
			fmt.Fprintf(c.ErrOrStderr(), "Imported into %s (%s)\n", t.db, t.service)
			p, _ := currentProject()
			if p == nil || !isProjectDB(p, t) {
				return nil
			}
			if noMagento, _ := c.Flags().GetBool("no-magento"); p.Type == "magento" && t.service != "mongodb" && !noMagento {
				if err := magentoAfterImport(p, t); err != nil {
					return err
				}
			}
			if noHooks, _ := c.Flags().GetBool("no-hooks"); noHooks {
				return nil
			}
			return runHooks(p, "post-db-import")
		})
	c.Flags().Bool("no-hooks", false, "do not run the post-db-import hooks")
	c.Flags().Bool("no-magento", false, "Magento: keep the dump's base URLs and cookie domain")
	return c
}

func init() { rootCmd.AddCommand(newDBExportCmd(), newDBImportCmd()) }
