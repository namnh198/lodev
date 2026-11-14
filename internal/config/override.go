package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// OverrideFiles lists .lodev/config.*.yaml (absolute, sorted by name, regular files only).
func OverrideFiles(root string) ([]string, error) {
	dir := filepath.Dir(ProjectPath(root))
	entries, err := os.ReadDir(dir) // sorted by name
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if ok, _ := filepath.Match("config.*.yaml", e.Name()); ok && e.Type().IsRegular() {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

// readOverride decodes one override file to its top-level mapping, nil when it holds nothing.
// Errors start with the file's path relative to the project root.
func readOverride(file string) (*yaml.Node, error) {
	label := ".lodev/" + filepath.Base(file)
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 || doc.Content[0].Tag == "!!null" {
		return nil, nil
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: must be a mapping of config keys", label)
	}
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == "name" {
			return nil, fmt.Errorf("%s: name cannot be overridden", label)
		}
	}
	return m, nil
}

// MergeOverrides applies every .lodev/config.*.yaml, in name order, over base: mappings merge
// key by key, anything else (lists included) is replaced. files are the overrides read.
func MergeOverrides(root string, base []byte) (merged []byte, files []string, err error) {
	files, err = OverrideFiles(root)
	if err != nil || len(files) == 0 {
		return base, files, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(base, &doc); err != nil {
		return nil, nil, fmt.Errorf(".lodev/config.yaml: %w", err)
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	for _, f := range files {
		m, err := readOverride(f)
		if err != nil {
			return nil, nil, err
		}
		if m != nil {
			mergeNode(doc.Content[0], m)
		}
	}
	merged, err = yaml.Marshal(&doc)
	return merged, files, err
}

func mergeNode(dst, src *yaml.Node) {
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		*dst = *src
		return
	}
next:
	for i := 0; i < len(src.Content); i += 2 {
		for j := 0; j < len(dst.Content); j += 2 {
			if dst.Content[j].Value == src.Content[i].Value {
				mergeNode(dst.Content[j+1], src.Content[i+1])
				continue next
			}
		}
		dst.Content = append(dst.Content, src.Content[i], src.Content[i+1])
	}
}

// OverriddenKeys maps each top-level key an override sets to the base name of the last file that sets it.
func OverriddenKeys(root string) (map[string]string, error) {
	files, err := OverrideFiles(root)
	if err != nil {
		return nil, err
	}
	keys := map[string]string{}
	for _, f := range files {
		m, err := readOverride(f)
		if err != nil {
			return nil, err
		}
		if m == nil {
			continue
		}
		for i := 0; i < len(m.Content); i += 2 {
			keys[m.Content[i].Value] = filepath.Base(f)
		}
	}
	return keys, nil
}
