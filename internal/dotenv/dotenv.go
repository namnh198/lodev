// Package dotenv reads and edits .env files: KEY=value lines, comments, quotes, export.
package dotenv

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Entry is one KEY=value line; Line is 1-based.
type Entry struct {
	Key, Value string
	Line       int
}

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Parse reads every KEY=value line; an error names its line.
func Parse(data []byte) ([]Entry, error) {
	var out []Entry
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := split(line)
		if !ok {
			return nil, fmt.Errorf("line %d: want KEY=value", i+1)
		}
		if !keyRe.MatchString(key) {
			return nil, fmt.Errorf("line %d: key %q is invalid", i+1, key)
		}
		v, err := value(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, Entry{key, v, i + 1})
	}
	return out, nil
}

// split cuts a trimmed line into its key and the raw text after '='.
func split(line string) (key, raw string, ok bool) {
	line = strings.TrimPrefix(line, "export ")
	k, r, ok := strings.Cut(line, "=")
	return strings.TrimSpace(k), strings.TrimSpace(r), ok
}

func value(raw string) (string, error) {
	switch {
	case strings.HasPrefix(raw, `"`):
		var b strings.Builder
		for i := 1; i < len(raw); i++ {
			c := raw[i]
			if c == '"' {
				return b.String(), trailing(raw[i+1:])
			}
			if c == '\\' && i+1 < len(raw) {
				i++
				switch raw[i] {
				case 'n':
					b.WriteByte('\n')
				case '"', '\\':
					b.WriteByte(raw[i])
				default:
					b.WriteByte('\\')
					b.WriteByte(raw[i])
				}
				continue
			}
			b.WriteByte(c)
		}
		return "", errors.New("unterminated double quote")
	case strings.HasPrefix(raw, "'"):
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", errors.New("unterminated single quote")
		}
		return raw[1 : end+1], trailing(raw[end+2:])
	}
	if v, _, found := strings.Cut(raw, " #"); found {
		return strings.TrimSpace(v), nil
	}
	return raw, nil
}

// trailing allows only a comment after a closing quote.
func trailing(rest string) error {
	if rest = strings.TrimSpace(rest); rest != "" && !strings.HasPrefix(rest, "#") {
		return fmt.Errorf("unexpected %q after the closing quote", rest)
	}
	return nil
}

// Get returns key's value; the last entry wins.
func Get(entries []Entry, key string) (string, bool) {
	v, ok := "", false
	for _, e := range entries {
		if e.Key == key {
			v, ok = e.Value, true
		}
	}
	return v, ok
}

// Set rewrites key's last line, or appends one; every other line is kept as is.
func Set(data []byte, key, val string) []byte {
	line := key + "=" + format(val)
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "#") {
			continue
		}
		if k, _, ok := split(t); ok && k == key {
			lines[i] = line
			return []byte(strings.Join(lines, "\n"))
		}
	}
	s := string(data)
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s + line + "\n")
}

func format(v string) string {
	if v != "" && !strings.ContainsAny(v, " #\"'$\\\n") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(v) + `"`
}
