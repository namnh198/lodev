package dotenv

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) []Entry {
	t.Helper()
	e, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestParseForms(t *testing.T) {
	in := "# c\n\nA=1\nexport B=two words \n C = 3\nD=\"x\\ny \\\"q\\\" \\\\\"\nE='lit $x \\n'\nF=v # note\nG=\nA=last\n"
	got := mustParse(t, in)
	want := [][2]string{{"A", "1"}, {"B", "two words"}, {"C", "3"}, {"D", "x\ny \"q\" \\"}, {"E", "lit $x \\n"}, {"F", "v"}, {"G", ""}, {"A", "last"}}
	if len(got) != len(want) {
		t.Fatalf("%d entries: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].Key != w[0] || got[i].Value != w[1] {
			t.Errorf("entry %d = %q=%q, want %q=%q", i, got[i].Key, got[i].Value, w[0], w[1])
		}
	}
	if v, ok := Get(got, "A"); v != "last" || !ok {
		t.Errorf("Get A = %q, %v", v, ok)
	}
	if _, ok := Get(got, "Z"); ok {
		t.Error("Get Z found")
	}
}

func TestParseErrors(t *testing.T) {
	for in, want := range map[string]string{"A=1\nnot a line\n": "line 2", "1A=x\n": "line 1", "A=\"open\n": "line 1"} {
		if _, err := Parse([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", in, err, want)
		}
	}
}

func TestParseCRLF(t *testing.T) {
	got := mustParse(t, "A=1\r\nB=\"x\"\r\n")
	if a, _ := Get(got, "A"); a != "1" {
		t.Errorf("A = %q", a)
	}
	if b, _ := Get(got, "B"); b != "x" {
		t.Errorf("B = %q", b)
	}
}

func TestSetReplacesInPlace(t *testing.T) {
	if got := string(Set([]byte("# keep\nA=1\nB=2\n"), "A", "9")); got != "# keep\nA=9\nB=2\n" {
		t.Errorf("%q", got)
	}
	if got := string(Set([]byte("A=1\nA=2\n"), "A", "9")); got != "A=1\nA=9\n" {
		t.Errorf("duplicate: %q", got)
	}
}

func TestSetAppends(t *testing.T) {
	if got := string(Set([]byte("A=1\n"), "B", "x")); got != "A=1\nB=x\n" {
		t.Errorf("%q", got)
	}
	if got := string(Set(nil, "B", "x")); got != "B=x\n" {
		t.Errorf("empty: %q", got)
	}
}

func TestSetNoTrailingNewline(t *testing.T) {
	if got := string(Set([]byte("A=1"), "B", "x")); got != "A=1\nB=x\n" {
		t.Errorf("%q", got)
	}
}

func TestSetQuotes(t *testing.T) {
	for _, v := range []string{"a b", "", "p$w", "h#1", "q\"x", "l1\nl2", "back\\s"} {
		out := Set(nil, "K", v)
		if !strings.HasPrefix(string(out), `K="`) {
			t.Errorf("%q not quoted: %q", v, out)
		}
		if got, _ := Get(mustParse(t, string(out)), "K"); got != v {
			t.Errorf("round trip %q → %q (%q)", v, got, out)
		}
	}
}
