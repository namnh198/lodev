package lodev

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/namnh198/lodev/pkg/fileutil"
	"github.com/namnh198/lodev/pkg/util"
)

// Composer runs Composer commands in the web container, managing pre- and post- hooks
// returns stdout, stderr, error
func (p *Project) Composer(args []string) (string, error) {

	stdout, err := p.Exec(&ExecOpts{
		Service: "web",
		Dir:     p.GetComposerRoot(true, true),
		RawCmd:  append([]string{"composer"}, args...),
		Tty:     isatty.IsTerminal(os.Stdin.Fd()),
		Env:     getComposerEnv(),
	})
	if err != nil {
		return stdout, fmt.Errorf("composer command failed: %v", err)
	}

	if util.IsWindows() {
		fileutil.ReplaceSimulatedLinks(p.AppRoot)
	}

	return stdout, nil
}

// getComposerEnv returns environment variables to use when running composer
func getComposerEnv() []string {
	env := []string{
		// Prevent Composer from debugging when Xdebug is enabled
		"XDEBUG_MODE=off",
	}

	// List of Composer environment variables to pass through from host
	// https://getcomposer.org/doc/03-cli.md#environment-variables
	composerEnvVars := []string{
		"COMPOSER_NO_SECURITY_BLOCKING",
	}

	for _, varName := range composerEnvVars {
		if value, exists := os.LookupEnv(varName); exists {
			env = append(env, fmt.Sprintf(`%s=%s`, varName, value))
		}
	}

	return env
}
