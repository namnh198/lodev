package cmd

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
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
	case openEnd == end: // self-closing
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
