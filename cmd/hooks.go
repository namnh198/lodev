package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// hookStep is one resolved step: the label lodev prints, and a command or a script path.
type hookStep struct {
	label, script string
	cmd           []string
}

// hookSteps lists event's steps: the config's first, then the event's script folder in name order.
func hookSteps(p *config.Project, event string) ([]hookStep, error) {
	var steps []hookStep
	for _, s := range p.Hooks[event] {
		if s.Script != "" {
			steps = append(steps, hookStep{label: s.Script, script: s.Script})
		} else {
			steps = append(steps, hookStep{label: s.Run, cmd: []string{"sh", "-c", s.Run}})
		}
	}
	rel := filepath.Join(".lodev", "hooks", event+".d")
	entries, err := os.ReadDir(filepath.Join(p.Root, rel))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries { // ReadDir sorts by name
		n := e.Name()
		if strings.HasPrefix(n, ".") || strings.HasSuffix(n, ".example") {
			continue
		}
		// Stat, not the entry's type: a symlink to a script counts.
		if fi, err := os.Stat(filepath.Join(p.Root, rel, n)); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		path := filepath.ToSlash(filepath.Join(rel, n))
		steps = append(steps, hookStep{label: path, script: path})
	}
	return steps, nil
}

// runHooks runs event's steps in the web container; the first failing step stops the rest.
func runHooks(p *config.Project, event string) error {
	steps, err := hookSteps(p, event)
	if err != nil || len(steps) == 0 {
		return err
	}
	if web := "lodev-" + p.Name + "-web"; !containerRunning(web) {
		return fmt.Errorf("hook %s: %s is not running", event, web)
	}
	for _, s := range steps {
		cmd := s.cmd
		if s.script != "" {
			fi, err := os.Stat(filepath.Join(p.Root, s.script))
			if err != nil {
				return fmt.Errorf("hook %s: %s does not exist", event, s.script)
			}
			// Its shebang picks the interpreter; lodev does not guess one for a plain file.
			if fi.Mode()&0o111 == 0 {
				return fmt.Errorf("hook %s: %s is not executable (chmod +x %s)", event, s.script, s.script)
			}
			cmd = []string{p.WorkDir() + "/" + s.script}
		}
		ui.Info("▶ %s: %s", event, s.label)
		full := append([]string{"env", "LODEV_HOOK=" + event, "LODEV_PROJECT=" + p.Name}, cmd...)
		if err := execInWeb(p, full); err != nil {
			var code interface{ ExitCode() int }
			if errors.As(err, &code) {
				return fmt.Errorf("hook %s failed: %s: exit status %d", event, s.label, code.ExitCode())
			}
			return fmt.Errorf("hook %s failed: %s: %w", event, s.label, err)
		}
	}
	return nil
}

// isProjectDB reports whether t is the project's own database: its DBName on its SQL service or MongoDB.
func isProjectDB(p *config.Project, t dbTarget) bool {
	return t.db == p.DBName() && (t.service == p.DB() || t.service == "mongodb" && p.Uses("mongodb"))
}
