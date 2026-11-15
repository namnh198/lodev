package config

import (
	"fmt"
	"path/filepath"
)

// NextjsDaemon is the implicit daemon of type nextjs; NextjsPort is where its dev server listens.
const (
	NextjsDaemon = "nextjs"
	NextjsPort   = 3000
)

// nodeLockfiles maps a lockfile to its package manager; the first present one wins.
var nodeLockfiles = []struct{ file, pm, install string }{
	{"pnpm-lock.yaml", "pnpm", "pnpm install --frozen-lockfile"},
	{"yarn.lock", "yarn", "yarn install"}, // yarn 1 and 4 name the frozen flag differently
	{"package-lock.json", "npm", "npm ci"},
}

// nodeInstall is the project's package manager and its install command, from its lockfile (pnpm without one).
func (p *Project) nodeInstall() (pm, install string) {
	for _, l := range nodeLockfiles {
		if exists(filepath.Join(p.Root, l.file)) {
			return l.pm, l.install
		}
	}
	return "pnpm", "pnpm install"
}

// NodePackageManager is pnpm, yarn or npm, from the project's lockfile; pnpm without one.
func (p *Project) NodePackageManager() string { pm, _ := p.nodeInstall(); return pm }

// NodeInstallCommand installs the project's packages: frozen to its lockfile when it has one.
func (p *Project) NodeInstallCommand() string { _, install := p.nodeInstall(); return install }

// Daemons is what supervisor runs: web_extra_daemons, then type nextjs's dev server.
func (p *Project) Daemons() []Daemon {
	if p.Type != "nextjs" {
		return p.WebExtraDaemons
	}
	pm, install := p.nodeInstall()
	// sh -c: the daemon script execs its command, so `a && b` needs a shell to reach b.
	cmd := fmt.Sprintf("sh -c '%s && PORT=%d exec %s run dev'", install, NextjsPort, pm)
	return append(append([]Daemon{}, p.WebExtraDaemons...), Daemon{Name: NextjsDaemon, Command: cmd})
}
