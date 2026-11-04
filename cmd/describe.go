package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/registry"
)

// serviceInfo is one of a project's own compose services, as docker ps reports it.
type serviceInfo struct{ Name, State, Ports, Image string }

// sharedPorts are the in-network ports of the shared services describe lists for a project.
var sharedPorts = map[string]string{"mysql": "3306", "mariadb": "3306", "redis": "6379", "opensearch": "9200", "rabbitmq": "5672", "mongodb": "27017"}

func stat(state string) string {
	switch state {
	case "running":
		return "OK"
	case "":
		return "stopped"
	}
	return state
}

// describeRows builds the SERVICE | STAT | URL/PORT | INFO rows; state gives a container's Docker state ("" = none).
func describeRows(p *config.Project, g *config.Global, state func(string) string, extra []serviceInfo) [][]string {
	var urls []string
	for _, d := range p.WebDomains() {
		urls = append(urls, "https://"+d)
	}

	web := []string{"InDocker: web"}
	if len(urls) > 0 {
		web = []string{urls[0], "InDocker: web:80"}
	}
	docroot := p.Docroot
	if docroot == "" {
		docroot = "."
	}
	info := []string{fmt.Sprintf("%s PHP %s", p.Type, p.PHP), "Server: " + p.Webserver, fmt.Sprintf("Docroot: '%s'", docroot)}
	if p.NodeJS != "" {
		info = append(info, "Node.js: "+p.NodeJS)
	}
	user, _ := p.ContainerUser()
	info = append(info, "User: "+user)
	rows := [][]string{{"web", stat(state("lodev-" + p.Name + "-web")), strings.Join(web, "\n"), strings.Join(info, "\n")}}

	if db := p.DB(); db != "" {
		port := "InDocker: " + db + ":3306"
		if host := map[string]string{"mysql": g.Ports.MySQL, "mariadb": g.Ports.MariaDB}[db]; host != "" {
			port += "\nHost: 127.0.0.1:" + host
		}
		rows = append(rows, []string{db, stat(state("lodev-" + db)), port,
			g.Image(db) + "\nUser/Pass: 'root/root'\nDatabase: " + p.DBName()})
	}
	if p.Uses("mongodb") {
		port := "InDocker: mongodb:" + sharedPorts["mongodb"]
		if g.Ports.MongoDB != "" {
			port += "\nHost: 127.0.0.1:" + g.Ports.MongoDB
		}
		rows = append(rows, []string{"mongodb", stat(state("lodev-mongodb")), port, g.Image("mongodb") + "\nUser/Pass: 'root/root'"})
	}
	for _, svc := range []string{"redis", "opensearch", "rabbitmq"} {
		if !p.Uses(svc) {
			continue
		}
		port, inf := "InDocker: "+svc+":"+sharedPorts[svc], g.Image(svc)
		if svc == "rabbitmq" {
			port, inf = "https://rabbitmq.test\n"+port, inf+"\nUser/Pass: 'guest/guest'"
		}
		rows = append(rows, []string{svc, stat(state("lodev-" + svc)), port, inf})
	}
	for _, s := range extra {
		port := "InDocker: " + s.Name
		if s.Ports != "" {
			port += "\n" + s.Ports
		}
		rows = append(rows, []string{s.Name, stat(s.State), port, s.Image})
	}

	if g.Services["phpmyadmin"] {
		rows = append(rows, []string{"phpMyAdmin", "", "https://phpmyadmin.test", ""})
	}
	if g.Services["mongo-express"] {
		rows = append(rows, []string{"Mongo Express", "", "https://mongo.test", ""})
	}
	if g.Services["mailpit"] {
		rows = append(rows, []string{"Mailpit", "", "https://mailpit.test\nSMTP: mailpit:1025", ""})
	}
	if len(urls) > 0 {
		rows = append(rows, []string{"Project URLs", "", strings.Join(urls, "\n"), ""})
	}
	return rows
}

// containerState is a container's Docker state, or "" when it does not exist (or Docker is down).
func containerState(name string) string {
	out, _ := dockerOutput("ps", "-a", "--filter", "name=^"+name+"$", "--format", "{{.State}}")
	return out
}

// projectExtraServices lists the project's compose services other than web, by name.
func projectExtraServices(p *config.Project) ([]serviceInfo, error) {
	out, err := dockerOutput("ps", "-a", "--filter", "label=com.docker.compose.project=lodev-"+p.Name,
		"--format", "{{.Label \"com.docker.compose.service\"}}\t{{.State}}\t{{.Ports}}\t{{.Image}}")
	if err != nil {
		return nil, err
	}
	var svcs []serviceInfo
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 || f[0] == "web" {
			continue
		}
		svcs = append(svcs, serviceInfo{f[0], f[1], f[2], f[3]})
	}
	slices.SortFunc(svcs, func(a, b serviceInfo) int { return strings.Compare(a.Name, b.Name) })
	return svcs, nil
}

// describeTarget is the named registry project, or the one in the current folder.
func describeTarget(args []string) (*config.Project, error) {
	if len(args) == 0 {
		return currentProject()
	}
	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}
	e, ok := reg[args[0]]
	if !ok {
		return nil, fmt.Errorf("unknown project %q (see lodev list)", args[0])
	}
	return config.LoadProject(e.Path)
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:     "describe [project]",
		Aliases: []string{"status", "st", "desc"},
		Short:   "Show a project's URLs, services and how to reach them",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := describeTarget(args)
			if err != nil {
				return err
			}
			g, err := config.LoadGlobal()
			if err != nil {
				return err
			}
			extra, _ := projectExtraServices(p) // Docker down: no extra rows, like the other states
			primary := ""
			if d := p.WebDomains(); len(d) > 0 {
				primary = " https://" + d[0]
			}
			w := c.OutOrStdout()
			fmt.Fprintf(w, "Project: %s %s%s\nDocker platform: %s\nRouter: traefik\nlodev version: %s\n",
				p.Name, shortPath(p.Root), primary, dockerValue("info", "--format", "{{.OperatingSystem}}"), lodevVersion())
			printTable(w, []string{"SERVICE", "STAT", "URL/PORT", "INFO"}, describeRows(p, g, containerState, extra))
			return nil
		},
	})
}
