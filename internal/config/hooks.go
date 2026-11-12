package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// HookEvents are the moments a project's hooks run.
var HookEvents = []string{"post-start", "pre-stop", "post-db-import", "post-snapshot-restore"}

// HookStep is one hook step: a shell command (a YAML string) or a project script ({script: path}).
type HookStep struct {
	Run, Script string
	badKey      string // a mapping key other than script; Validate reports it
}

func (h *HookStep) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		h.Run = n.Value
		return nil
	}
	var m map[string]string
	if err := n.Decode(&m); err != nil {
		return err
	}
	for k, v := range m {
		if k != "script" {
			h.badKey = k
		}
		if k == "script" {
			h.Script = v
		}
	}
	return nil
}

func (h HookStep) MarshalYAML() (any, error) {
	if h.Script != "" {
		return map[string]string{"script": h.Script}, nil
	}
	return h.Run, nil
}

// insideProject reports whether rel is a relative path that stays in the project folder.
func insideProject(rel string) bool {
	c := filepath.Clean(rel)
	return !filepath.IsAbs(rel) && c != ".." && !strings.HasPrefix(c, "../")
}

func validateHooks(hooks map[string][]HookStep) error {
	for _, ev := range slices.Sorted(maps.Keys(hooks)) {
		if !slices.Contains(HookEvents, ev) {
			return fmt.Errorf("hooks: unknown event %q (allowed: %s)", ev, strings.Join(HookEvents, ", "))
		}
		for i, s := range hooks[ev] {
			switch {
			case s.badKey != "":
				return fmt.Errorf(`hooks.%s[%d]: only "script" is allowed`, ev, i)
			case s.Script != "":
				if !insideProject(s.Script) {
					return fmt.Errorf("hooks.%s[%d]: script %q must be a file inside the project", ev, i, s.Script)
				}
			case strings.TrimSpace(s.Run) == "":
				return fmt.Errorf("hooks.%s[%d] is empty", ev, i)
			}
		}
	}
	return nil
}
