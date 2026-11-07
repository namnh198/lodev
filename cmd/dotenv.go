package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/dotenv"
)

// flagKey turns a DDEV-style flag into a variable name: --app-key → APP_KEY.
func flagKey(flag string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimPrefix(flag, "--"), "-", "_"))
}

func wantsHelp(args []string) bool {
	return slices.Contains(args, "-h") || slices.Contains(args, "--help")
}

func readProjectDotenv() (*config.Project, []byte, error) {
	p, err := currentProject()
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(dotenvPath(p))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	return p, data, nil
}

func newDotenvCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "dotenv set|get",
		Short: "Set or read variables in .lodev/.env (the web container's local env)",
	}
	c.AddCommand(&cobra.Command{
		Use:                "set --key=value [...]",
		Short:              "Set variables in .lodev/.env: --app-key=abc writes APP_KEY=abc",
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			if wantsHelp(args) {
				return c.Help()
			}
			if len(args) == 0 {
				return errors.New("usage: lodev dotenv set --key=value [...]")
			}
			type kv struct{ k, v string }
			var sets []kv
			for _, a := range args {
				flag, v, ok := strings.Cut(a, "=")
				if !strings.HasPrefix(flag, "--") || len(flag) < 3 {
					return errors.New("usage: lodev dotenv set --key=value [...]")
				}
				if !ok {
					return fmt.Errorf("%s needs a value (%s=value)", flag, flag)
				}
				k := flagKey(flag)
				if err := config.CheckEnvKey(k); err != nil {
					return err
				}
				sets = append(sets, kv{k, v})
			}
			p, data, err := readProjectDotenv()
			if err != nil {
				return err
			}
			// An older .lodev/.gitignore may predate .env: keep secrets out of git before writing.
			if err := writeLodevGitignore(p); err != nil {
				return err
			}
			keys := make([]string, len(sets))
			for i, s := range sets {
				data = dotenv.Set(data, s.k, s.v)
				keys[i] = s.k
			}
			if err := os.WriteFile(dotenvPath(p), data, 0o600); err != nil {
				return err
			}
			if err := os.Chmod(dotenvPath(p), 0o600); err != nil { // WriteFile keeps an existing file's mode
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Set %s in .lodev/.env; run lodev restart to apply\n", strings.Join(keys, ", "))
			return nil
		},
	}, &cobra.Command{
		Use:                "get --key",
		Short:              "Print a variable from .lodev/.env: --app-key prints APP_KEY",
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			if wantsHelp(args) {
				return c.Help()
			}
			if len(args) != 1 || !strings.HasPrefix(args[0], "--") || strings.Contains(args[0], "=") {
				return errors.New("usage: lodev dotenv get --key")
			}
			_, data, err := readProjectDotenv()
			if err != nil {
				return err
			}
			entries, err := dotenv.Parse(data)
			if err != nil {
				return fmt.Errorf(".lodev/.env: %w", err)
			}
			k := flagKey(args[0])
			v, ok := dotenv.Get(entries, k)
			if !ok {
				return fmt.Errorf("%s is not set in .lodev/.env", k)
			}
			fmt.Fprintln(c.OutOrStdout(), v)
			return nil
		},
	})
	return c
}

func init() { rootCmd.AddCommand(newDotenvCmd()) }
