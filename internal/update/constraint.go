package update

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	versionRe = regexp.MustCompile(`^v?\d+(\.\d+)*$`)
	// releaseRe is a release tag, or a nightly build of one: v1.2.3-4-gabcdef.
	releaseRe = regexp.MustCompile(`^(v\d+\.\d+\.\d+)(-\d+-g[0-9a-f]+)?$`)
	clauseRe  = regexp.MustCompile(`^(>=|<=|!=|>|<|=)\s*(v?\d+(\.\d+)*)$`)
)

// Compare orders two dotted versions, with or without a leading v ("1.10" > "v1.2.0"); a missing part is 0.
func Compare(a, b string) (int, error) {
	for _, v := range []string{a, b} {
		if !versionRe.MatchString(v) {
			return 0, fmt.Errorf("%q is not a version like v1.2.3", v)
		}
	}
	x, y := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < max(len(x), len(y)); i++ {
		var m, n int
		if i < len(x) {
			m, _ = strconv.Atoi(x[i])
		}
		if i < len(y) {
			n, _ = strconv.Atoi(y[i])
		}
		if m != n {
			if m < n {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

// ReleaseBase is the release a build is made from: v1.2.3 for v1.2.3 and v1.2.3-4-gabcdef; false for dev builds.
func ReleaseBase(v string) (string, bool) {
	m := releaseRe.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Constraint is a comma-separated list of clauses that must all hold, e.g. ">= 1.4.0, < 2".
type Constraint []struct{ op, version string }

// ParseConstraint reads clauses of >=, <=, >, <, = or != followed by a version.
func ParseConstraint(s string) (Constraint, error) {
	var c Constraint
	for _, part := range strings.Split(s, ",") {
		m := clauseRe.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil, fmt.Errorf("%q is not a comparison like >= 1.4.0 (use >=, <=, >, <, = or !=)", strings.TrimSpace(part))
		}
		c = append(c, struct{ op, version string }{m[1], m[2]})
	}
	return c, nil
}

// Allows reports whether version v satisfies every clause; a v that is not a version never does.
func (c Constraint) Allows(v string) bool {
	for _, cl := range c {
		got, err := Compare(v, cl.version)
		if err != nil {
			return false
		}
		ok := false
		switch cl.op {
		case ">=":
			ok = got >= 0
		case "<=":
			ok = got <= 0
		case ">":
			ok = got > 0
		case "<":
			ok = got < 0
		case "=":
			ok = got == 0
		case "!=":
			ok = got != 0
		}
		if !ok {
			return false
		}
	}
	return true
}
