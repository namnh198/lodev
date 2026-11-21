package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// NextjsDaemon is the implicit daemon of type nextjs; NextjsPort is its dev server's default port.
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
	port := p.EffectiveContainerPort()
	// sh -c: the daemon script execs its command, so `a && b` needs a shell to reach b.
	// NODE_ENV is read in the container (config.yaml or .lodev/.env); the install keeps
	// devDependencies even under production, since next build needs them (typescript).
	cmd := fmt.Sprintf(`sh -c 'NODE_ENV=development %s && if [ "$NODE_ENV" = production ]; then %s run build && PORT=%d exec %s run %s; else PORT=%d exec %s run dev; fi'`,
		install, pm, port, pm, p.NextjsProdScript(), port, pm)
	return append(append([]Daemon{}, p.WebExtraDaemons...), Daemon{Name: NextjsDaemon, Command: cmd})
}

// EffectiveContainerPort is container_port, else NextjsPort for type nextjs, else 80.
func (p *Project) EffectiveContainerPort() int {
	switch {
	case p.ContainerPort != 0:
		return p.ContainerPort
	case p.Type == "nextjs":
		return NextjsPort
	}
	return 80
}

// NodeScripts is package.json's scripts, or nil when it is missing or does not parse.
func (p *Project) NodeScripts() map[string]string {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	b, err := os.ReadFile(filepath.Join(p.Root, "package.json"))
	if err != nil || json.Unmarshal(b, &pkg) != nil {
		return nil
	}
	return pkg.Scripts
}

// NextjsProdScript is what NODE_ENV=production runs after build: start, or preview when only that exists.
func (p *Project) NextjsProdScript() string {
	s := p.NodeScripts()
	if _, ok := s["start"]; !ok && s["preview"] != "" {
		return "preview"
	}
	return "start"
}
