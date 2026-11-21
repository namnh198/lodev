package cmd

import (
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"lodev/internal/config"
	"lodev/internal/ui"
)

// ideaSkeleton is an empty PhpStorm project file, for a .idea file that does not exist yet.
const ideaSkeleton = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<project version=\"4\">\n</project>\n"

var xmlAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// xmlAttr escapes s for an XML attribute value.
func xmlAttr(s string) string { return xmlAttrEscaper.Replace(s) }

// newUUID is a random (version 4) UUID, as PhpStorm uses for interpreter and server ids.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// findComponent locates <component name="name" ...>: the open tag's span and the end of the
// whole element. ok is false when the file has no such component.
func findComponent(doc, name string) (start, openEnd, end int, ok bool) {
	// The closing quote keeps PhpInterpreters from matching PhpInterpretersPhpInfoCache.
	start = strings.Index(doc, `<component name="`+xmlAttr(name)+`"`)
	if start < 0 {
		return 0, 0, 0, false
	}
	openEnd = start + strings.Index(doc[start:], ">") + 1
	if doc[openEnd-2] == '/' {
		return start, openEnd, openEnd, true
	}
	return start, openEnd, openEnd + strings.Index(doc[openEnd:], "</component>"), true
}

// ideaComplete is whether doc is well-formed XML with a closing </project>, which the splicing relies on.
func ideaComplete(doc string) bool {
	if !strings.Contains(doc, "</project>") {
		return false
	}
	d := xml.NewDecoder(strings.NewReader(doc))
	for {
		if _, err := d.Token(); err != nil {
			return err == io.EOF
		}
	}
}

// insertBeforeProjectEnd adds text (whole lines) just before </project>.
func insertBeforeProjectEnd(doc, text string) string {
	i := strings.LastIndex(doc, "</project>")
	return doc[:i] + text + doc[i:]
}

// lineStart moves i back over the indentation in front of it.
func lineStart(s string, i int) int {
	for i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
		i--
	}
	return i
}

// ensureComponentList makes sure the component exists and holds an open <list>...</list>.
func ensureComponentList(doc, component, list string) string {
	empty := "    <" + list + ">\n    </" + list + ">\n"
	start, openEnd, end, ok := findComponent(doc, component)
	switch {
	case !ok:
		return insertBeforeProjectEnd(doc, `  <component name="`+xmlAttr(component)+"\">\n"+empty+"  </component>\n")
	case doc[openEnd-2] == '/': // self-closing, not merely empty
		open := strings.TrimSuffix(strings.TrimSuffix(doc[start:openEnd], "/>"), " ") + ">"
		return doc[:start] + open + "\n" + empty + "  </component>" + doc[end:]
	}
	body := doc[openEnd:end]
	if strings.Contains(body, "</"+list+">") {
		return doc
	}
	if i := strings.Index(body, "<"+list+" />"); i >= 0 {
		return doc[:openEnd] + body[:i] + "<" + list + ">\n    </" + list + ">" + body[i+len(list)+4:] + doc[end:]
	}
	at := lineStart(doc, end)
	return doc[:at] + empty + doc[at:]
}

var idAttr = regexp.MustCompile(` id="([^"]*)"`)

// findElement locates the <tag ... name="name"> element in body, as whole lines; id is its id attribute.
func findElement(body, tag, name string) (start, end int, id string, ok bool) {
	for from := 0; ; {
		i := strings.Index(body[from:], "<"+tag+" ")
		if i < 0 {
			return 0, 0, "", false
		}
		i += from
		openEnd := i + strings.Index(body[i:], ">") + 1
		open := body[i:openEnd]
		from = openEnd
		if !strings.Contains(open, ` name="`+xmlAttr(name)+`"`) {
			continue
		}
		end = openEnd
		if !strings.HasSuffix(open, "/>") {
			end = openEnd + strings.Index(body[openEnd:], "</"+tag+">") + len("</"+tag+">")
		}
		if end < len(body) && body[end] == '\n' {
			end++
		}
		if m := idAttr.FindStringSubmatch(open); m != nil {
			id = m[1]
		}
		return lineStart(body, i), end, id, true
	}
}

// upsertElement replaces the <tag name="name"> element inside component's <list> with build(its id),
// or appends build(a new id) to the list. build returns whole indented lines. Nothing else changes.
func upsertElement(doc, component, list, tag, name string, build func(id string) string) string {
	doc = ensureComponentList(doc, component, list)
	_, openEnd, end, _ := findComponent(doc, component)
	body := doc[openEnd:end]
	if s, e, id, ok := findElement(body, tag, name); ok {
		return doc[:openEnd] + body[:s] + build(id) + body[e:] + doc[end:]
	}
	at := lineStart(body, strings.Index(body, "</"+list+">"))
	return doc[:openEnd] + body[:at] + build(newUUID()) + body[at:] + doc[end:]
}

// setComponentAttr sets attr="value" on component's open tag, adding the component when missing.
func setComponentAttr(doc, component, attr, value string) string {
	pair := attr + `="` + xmlAttr(value) + `"`
	start, openEnd, _, ok := findComponent(doc, component)
	if !ok {
		return insertBeforeProjectEnd(doc, `  <component name="`+xmlAttr(component)+`" `+pair+" />\n")
	}
	open := doc[start:openEnd]
	re := regexp.MustCompile(` ` + regexp.QuoteMeta(attr) + `="[^"]*"`)
	if re.MatchString(open) {
		open = re.ReplaceAllLiteralString(open, " "+pair)
	} else if head, ok := strings.CutSuffix(open, " />"); ok {
		open = head + " " + pair + " />"
	} else {
		open = strings.TrimSuffix(open, ">") + " " + pair + ">"
	}
	return doc[:start] + open + doc[openEnd:]
}

// phpstormRunning is whether a PhpStorm process exists; -x matches the name, never `lodev phpstorm` itself.
var phpstormRunning = func() bool { return exec.Command("pgrep", "-ix", "phpstorm").Run() == nil }

// phpstormOpen opens PhpStorm on the project root.
var phpstormOpen = func(root string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-a", "PhpStorm", root).Run()
	}
	return exec.Command("phpstorm", root).Start()
}

// phpstormInterpreter is the project's Docker Compose CLI interpreter, as PhpStorm writes it.
func phpstormInterpreter(p *config.Project) func(id string) string {
	return func(id string) string {
		return `      <interpreter id="` + xmlAttr(id) + `" name="lodev" home="docker-compose://DATA" auto="false" debugger_id="php.debugger.XDebug">
        <remote_data INTERPRETER_PATH="/usr/bin/php` + xmlAttr(p.PHP) + `" HELPERS_PATH="/opt/.phpstorm_helpers" VALID="true" RUN_AS_ROOT_VIA_SUDO="false" DOCKER_ACCOUNT_NAME="Docker" DOCKER_COMPOSE_SERVICE_NAME="web" DOCKER_REMOTE_PROJECT_PATH="/opt/project">
          <type_data command="EXEC" />
          <dockerComposeConfigurationPaths>
            <item value="$PROJECT_DIR$/.lodev/lodev.compose-full.yml" />
          </dockerComposeConfigurationPaths>
          <envs />
        </remote_data>
      </interpreter>
`
	}
}

// phpstormServer is the debug server PHP_IDE_CONFIG names, mapping the project to working_dir.
func phpstormServer(p *config.Project) func(id string) string {
	return func(id string) string {
		return `      <server host="` + xmlAttr(p.Domains[0]) + `" id="` + xmlAttr(id) + `" name="` + xmlAttr(p.IDEServerName()) + `" use_path_mappings="true">
        <path_mappings>
          <mapping local-root="$PROJECT_DIR$" remote-root="` + xmlAttr(p.WorkDir()) + `" />
        </path_mappings>
      </server>
`
	}
}

// writePhpstorm writes lodev's interpreter, debug server and selected interpreter into .idea/,
// leaving every other setting as it is.
func writePhpstorm(p *config.Project, yes bool) error {
	if p.Type == "nextjs" {
		return errors.New("nextjs projects have no php interpreter for phpstorm")
	}
	if _, err := os.Stat(composeFull(p)); err != nil {
		return errors.New(".lodev/lodev.compose-full.yml is missing: run lodev start first")
	}
	if phpstormRunning() {
		ui.Warn("PhpStorm is running and may overwrite .idea/workspace.xml when it saves")
		if !confirm("Write .idea anyway?", yes) {
			return errors.New("phpstorm is running: close the project first, or pass --yes")
		}
	}
	dir := filepath.Join(p.Root, ".idea")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	edits := []struct {
		name string
		edit func(string) string
	}{
		{"php.xml", func(doc string) string {
			return upsertElement(doc, "PhpInterpreters", "interpreters", "interpreter", "lodev", phpstormInterpreter(p))
		}},
		{"workspace.xml", func(doc string) string {
			doc = upsertElement(doc, "PhpServers", "servers", "server", p.IDEServerName(), phpstormServer(p))
			return setComponentAttr(doc, "PhpWorkspaceProjectConfiguration", "interpreter_name", "lodev")
		}},
	}
	// Both files are checked and edited before either is written, so a broken one leaves both as they were.
	outs := map[string]string{}
	for _, e := range edits {
		path := filepath.Join(dir, e.name)
		doc := ideaSkeleton
		if b, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(b)) != "" {
			doc = string(b)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if !ideaComplete(doc) {
			return fmt.Errorf(".idea/%s is not a complete PhpStorm file: fix or delete it, then run lodev phpstorm again", e.name)
		}
		if out := e.edit(doc); out != doc {
			outs[path] = out
		}
	}
	for path, out := range outs {
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			return err
		}
	}
	ui.Success("PhpStorm: interpreter lodev (php%s), server %s → %s; reopen the project if it is open", p.PHP, p.IDEServerName(), p.WorkDir())
	return nil
}

func init() {
	var yes bool
	c := &cobra.Command{
		Use:   "phpstorm",
		Short: "Write the PhpStorm interpreter, debug server and path mapping into .idea/, then open PhpStorm",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			p, err := currentProject()
			if err != nil {
				return err
			}
			if err := writePhpstorm(p, yes); err != nil {
				return err
			}
			if err := phpstormOpen(p.Root); err != nil {
				ui.Warn(".idea/ is set up; could not open PhpStorm (%v): open the project yourself", err)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "do not ask while PhpStorm is running")
	rootCmd.AddCommand(c)
}
