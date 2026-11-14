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

// dbNameRe is what lodev puts between backticks in SQL and passes to the clients without escaping:
// no leading - (mysqldump would read -A as an option), no $ (invalid in MongoDB).
var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

// mongoURI is the shared MongoDB's root login (MONGO_INITDB_ROOT_* in the services compose).
const mongoURI = "--uri=mongodb://root:root@localhost:27017/?authSource=admin"

type dbTarget struct{ service, db string }

// dbTargetFor picks the service (-s, else the project's database) and the database (--db-name or
// the argument, else the project's).
func dbTargetFor(args []string, service, name string) (dbTarget, error) {
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
	given := name != ""
	if len(args) > 0 {
		if name != "" && name != args[0] {
			return dbTarget{}, fmt.Errorf("the database is %q as an argument and %q with --db-name; give one", args[0], name)
		}
		name, given = args[0], true
	}
	switch {
	case given:
		if !dbNameRe.MatchString(name) {
			return dbTarget{}, fmt.Errorf("database name %q: use A-Z a-z 0-9 _ - (not first)", name)
		}
		t.db = name
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

// dbDumpArgs is the dump command; extra (--extra-command) goes before the SQL database name, or
// at the end for MongoDB.
func dbDumpArgs(t dbTarget, extra []string) []string {
	opts := []string{"-uroot", "--single-transaction", "--routines", "--triggers", "--events", "--no-tablespaces"}
	switch t.service {
	case "mongodb":
		return dbExecArgs(t, append([]string{"mongodump", mongoURI, "--db=" + t.db, "--archive"}, extra...)...)
	case "mariadb":
		return dbExecArgs(t, sqlCmd("mariadb-dump", opts, extra, t.db)...)
	}
	return dbExecArgs(t, sqlCmd("mysqldump", opts, extra, t.db)...)
}

func dbRestoreArgs(t dbTarget, extra []string) []string {
	switch t.service {
	case "mongodb":
		// Any source database lands in t.db; --drop replaces its collections, like a SQL dump's DROP TABLE.
		return dbExecArgs(t, append([]string{"mongorestore", mongoURI, "--archive", "--drop", "--nsFrom=$db$.$col$", "--nsTo=" + t.db + ".$col$"}, extra...)...)
	case "mariadb":
		return dbExecArgs(t, sqlCmd("mariadb", []string{"-uroot"}, extra, t.db)...) // MariaDB 11 has no mysql client
	}
	return dbExecArgs(t, sqlCmd("mysql", []string{"-uroot"}, extra, t.db)...)
}

// sqlCmd is client opts... extra... db: the clients read options before the database name.
func sqlCmd(client string, opts, extra []string, db string) []string {
	return append(append(append([]string{client}, opts...), extra...), db)
}

// splitArgs splits --extra-command like a shell: words split on whitespace; '...' is literal;
// "..." keeps spaces and takes \" and \\ escapes; \ outside quotes escapes the next character.
// Nothing is expanded or run: the words reach the client as arguments.
func splitArgs(s string) ([]string, error) {
	var out []string
	var w strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				out, inWord = append(out, w.String()), false
				w.Reset()
			}
			continue
		case c == '\\':
			if i++; i == len(s) {
				return nil, errors.New("--extra-command: trailing backslash")
			}
			w.WriteByte(s[i])
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return nil, errors.New("--extra-command: unclosed quote")
			}
			w.WriteString(s[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			for i++; ; i++ {
				if i == len(s) {
					return nil, errors.New("--extra-command: unclosed quote")
				}
				if s[i] == '"' {
					break
				}
				if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
					i++
				}
				w.WriteByte(s[i])
			}
		default:
			w.WriteByte(c)
		}
		inWord = true
	}
	if inWord {
		out = append(out, w.String())
	}
	return out, nil
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
func dumpTo(t dbTarget, w io.Writer, gz bool, extra []string) error {
	if !gz {
		return dockerStream(dbDumpArgs(t, extra), nil, w)
	}
	zw := gzip.NewWriter(w)
	if err := dockerStream(dbDumpArgs(t, extra), nil, zw); err != nil {
		return err
	}
	return zw.Close()
}

// restoreFrom loads a plain or gzipped dump into t, creating the SQL database first. Shared by
// db-import and snapshot.
func restoreFrom(t dbTarget, r io.Reader, extra []string) error {
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
	return dockerStream(dbRestoreArgs(t, extra), r, nil)
}

// dbFlags reads -s and -c and resolves the target, checking its container runs.
func dbFlags(c *cobra.Command, args []string) (dbTarget, bool, error) {
	service, _ := c.Flags().GetString("service")
	compress, _ := c.Flags().GetString("compress")
	if compress != "" && compress != "gz" {
		return dbTarget{}, false, fmt.Errorf("--compress %q: only gz is supported", compress)
	}
	name, _ := c.Flags().GetString("db-name")
	t, err := dbTargetFor(args, service, name)
	if err != nil {
		return dbTarget{}, false, err
	}
	if !containerRunning("lodev-" + t.service) {
		return dbTarget{}, false, fmt.Errorf("lodev-%s is not running (lodev service enable %s)", t.service, t.service)
	}
	return t, compress == "gz", nil
}

func dbCmd(use, short string, run func(c *cobra.Command, t dbTarget, gz bool, file string, extra []string) error) *cobra.Command {
	c := &cobra.Command{
		Use:   use + " [database]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// Before Docker or the dump file: a bad quote must not leave an empty file behind.
			var extra []string
			opts, _ := c.Flags().GetStringArray("extra-command")
			for _, o := range opts {
				words, err := splitArgs(o)
				if err != nil {
					return err
				}
				extra = append(extra, words...)
			}
			t, gz, err := dbFlags(c, args)
			if err != nil {
				return err
			}
			file, _ := c.Flags().GetString("file")
			return run(c, t, gz, file, extra)
		},
	}
	c.Flags().StringP("service", "s", "", "mysql | mariadb | mongodb (default: the project's database)")
	c.Flags().StringP("db-name", "n", "", "the database (default: the project's); same as the [database] argument")
	c.Flags().StringP("compress", "c", "", "gz: gzip the dump (import detects gzip by itself)")
	c.Flags().StringP("file", "f", "", "dump file; - for stdout/stdin")
	c.Flags().StringArrayP("extra-command", "e", nil, `more options for the dump/restore client, split like a shell (e.g. -e '--skip-add-locks'); repeatable`)
	return c
}

func newDBExportCmd() *cobra.Command {
	return dbCmd("db-export", "Dump a database of the shared mysql, mariadb or mongodb (default: the project's)",
		func(c *cobra.Command, t dbTarget, gz bool, file string, extra []string) error {
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
			err := dumpTo(t, w, gz, extra)
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
		func(c *cobra.Command, t dbTarget, _ bool, file string, extra []string) error {
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
			if err := restoreFrom(t, r, extra); err != nil {
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
			return runHooks(p, "post-db-import", false)
		})
	c.Flags().Bool("no-hooks", false, "do not run the post-db-import hooks")
	c.Flags().Bool("no-magento", false, "Magento: keep the dump's base URLs and cookie domain")
	return c
}

func init() { rootCmd.AddCommand(newDBExportCmd(), newDBImportCmd()) }
