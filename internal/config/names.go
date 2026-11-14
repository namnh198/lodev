package config

import "strings"

// Docker names lodev gives a project's compose stack and containers; the compose templates use them too.

func (p *Project) ComposeProject() string   { return "lodev-" + p.Name }
func (p *Project) WebContainer() string     { return WebContainer(p.Name) }
func (p *Project) VarnishContainer() string { return "lodev-" + p.Name + "-varnish" }

// WebContainer is the web container of the project named project.
func WebContainer(project string) string { return "lodev-" + project + "-web" }

// ServiceContainer is a shared service's container, such as lodev-mysql.
func ServiceContainer(svc string) string { return "lodev-" + svc }

// ProjectOfWebContainer is the project name in a web container's name.
func ProjectOfWebContainer(c string) string {
	return strings.TrimSuffix(strings.TrimPrefix(c, "lodev-"), "-web")
}
