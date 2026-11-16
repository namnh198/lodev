package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// nextjsDevScript is package.json's scripts.dev, or "" when there is none (or it does not parse).
func nextjsDevScript(root string) string {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	b, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil || json.Unmarshal(b, &pkg) != nil {
		return ""
	}
	return pkg.Scripts["dev"]
}

// warnNextjs says what a nextjs project's setup will do to the host or what it still needs.
func warnNextjs(p *config.Project, g *config.Global) {
	if p.Type != "nextjs" {
		return
	}
	// A Linux host can use the container's Linux packages, so only other hosts are warned.
	if p.EffectivePerformanceMode(g) == "none" && setupOS != "linux" {
		var shared []string
		for _, d := range []string{"node_modules", ".next"} {
			if fi, err := os.Stat(filepath.Join(p.Root, d)); err == nil && fi.IsDir() {
				shared = append(shared, d)
			}
		}
		if len(shared) > 0 {
			ui.Warn("performance_mode is none: the container installs Linux packages into the host's %s; run lodev config --performance-mode=bind to keep them in a volume", strings.Join(shared, ", "))
		}
	}
	if len(p.Domains) == 0 {
		return
	}
	for _, f := range []string{"next.config.ts", "next.config.js", "next.config.mjs"} {
		b, err := os.ReadFile(filepath.Join(p.Root, f))
		if err != nil {
			continue
		}
		// shortcut: a substring check, not a parse; a domain only in a comment counts as set.
		if !strings.Contains(string(b), p.Domains[0]) {
			ui.Warn("%s does not allow %s: Next.js blocks its dev assets there; add allowedDevOrigins: [%q] to it", f, p.Domains[0], p.Domains[0])
		}
		return
	}
}
