package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Doc is a YAML file opened for editing. Only the keys that are set or deleted change;
// comments, key order and list style of everything else are kept.
type Doc struct {
	path string
	root *yaml.Node // the top-level mapping
	head *yaml.Node // the document node
	mode fs.FileMode
}

// OpenDoc reads path; a missing, empty or comment-only file is an empty mapping.
func OpenDoc(path string) (*Doc, error) {
	d := &Doc{path: path, mode: 0o644}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if st, err := os.Stat(path); err == nil {
			d.mode = st.Mode().Perm()
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind == 0 { // empty file; keep a comment-only file's comment
		doc = yaml.Node{Kind: yaml.DocumentNode, HeadComment: doc.HeadComment}
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: not a YAML mapping", path)
	}
	d.head, d.root = &doc, doc.Content[0]
	return d, nil
}

// lookup returns the index of key's key node in mapping m, or -1.
func lookup(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func valueNode(v any) *yaml.Node {
	switch v := v.(type) {
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
	case []string:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
		for _, s := range v {
			n.Content = append(n.Content, valueNode(s))
		}
		return n
	}
	panic(fmt.Sprintf("config.Doc: unsupported value %T", v))
}

// Set sets the value at path (string, int, bool or []string), creating parent maps.
func (d *Doc) Set(path []string, v any) {
	m := d.root
	for _, k := range path[:len(path)-1] {
		i := lookup(m, k)
		if i < 0 || m.Content[i+1].Kind != yaml.MappingNode {
			child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			if i < 0 {
				m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, child)
			} else {
				m.Content[i+1] = child
			}
		}
		m = m.Content[lookup(m, k)+1]
	}
	n := valueNode(v)
	key := path[len(path)-1]
	i := lookup(m, key)
	if i < 0 {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, n)
		return
	}
	old := m.Content[i+1]
	n.LineComment = old.LineComment
	if old.Kind == yaml.SequenceNode && n.Kind == yaml.SequenceNode {
		n.Style = old.Style
	}
	m.Content[i+1] = n
}

// Delete removes the key at path; a missing key is a no-op.
func (d *Doc) Delete(path []string) {
	m := d.root
	for _, k := range path[:len(path)-1] {
		i := lookup(m, k)
		if i < 0 || m.Content[i+1].Kind != yaml.MappingNode {
			return
		}
		m = m.Content[i+1]
	}
	i := lookup(m, path[len(path)-1])
	if i < 0 {
		return
	}
	head := m.Content[i].HeadComment
	m.Content = append(m.Content[:i], m.Content[i+2:]...)
	// yaml.v3 attaches a file's top comment to the first key; keep it on the new first key.
	if i == 0 && head != "" && len(m.Content) > 0 {
		if c := m.Content[0].HeadComment; c != "" {
			head += "\n" + c
		}
		m.Content[0].HeadComment = head
	}
}

// Bytes encodes the document with 2-space indentation.
func (d *Doc) Bytes() ([]byte, error) {
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(d.head); err != nil {
		return nil, err
	}
	if err := e.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Write replaces the file with data atomically, keeping its mode (0644 for a new file).
func (d *Doc) Write(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(d.path), 0o755); err != nil {
		return err
	}
	tmp := d.path + ".tmp"
	if err := os.WriteFile(tmp, data, d.mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, d.mode); err != nil { // WriteFile keeps an existing tmp's mode
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, d.path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
