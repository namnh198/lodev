package update

import "testing"

func TestCompare(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{{"v1.2.0", "1.10", -1}, {"1", "v1.0.0", 0}, {"v2.0.1", "v2.0.0", 1}} {
		if got, err := Compare(tt.a, tt.b); err != nil || got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d", tt.a, tt.b, got, err, tt.want)
		}
	}
	if _, err := Compare("x", "1"); err == nil {
		t.Error("Compare(x, 1): want an error")
	}
}

func TestParseConstraint(t *testing.T) {
	for _, s := range []string{">= 1.4.0, < 2", ">=1.4"} {
		if _, err := ParseConstraint(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"1.4", ">= 1.4,", "=> 1", ""} {
		if _, err := ParseConstraint(s); err == nil {
			t.Errorf("%q: want an error", s)
		}
	}
}

func TestAllows(t *testing.T) {
	for _, tt := range []struct {
		c, v string
		want bool
	}{
		{">= 1.4.0, < 2", "v1.4.0", true},
		{">= 1.4.0, < 2", "v1.9.9", true},
		{">= 1.4.0, < 2", "v1.3.9", false},
		{">= 1.4.0, < 2", "v2.0.0", false},
		{"!= 1.5.0", "v1.5.0", false},
		{"!= 1.5.0", "v1.5.1", true},
		{"= 1.5", "v1.5.0", true},
		{">= 1", "dev", false},
	} {
		c, err := ParseConstraint(tt.c)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Allows(tt.v); got != tt.want {
			t.Errorf("%q allows %q = %v", tt.c, tt.v, got)
		}
	}
}

func TestReleaseBase(t *testing.T) {
	for _, tt := range []struct {
		v, want string
		ok      bool
	}{
		{"v1.2.3", "v1.2.3", true},
		{"v0.1.1-3-g238f647", "v0.1.1", true},
		{"dev", "", false},
		{"dev (abc-dirty)", "", false},
	} {
		if got, ok := ReleaseBase(tt.v); got != tt.want || ok != tt.ok {
			t.Errorf("ReleaseBase(%q) = %q, %v", tt.v, got, ok)
		}
	}
}
