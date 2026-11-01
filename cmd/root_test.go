package cmd

import (
	"reflect"
	"testing"
)

func TestStripDebugArg(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
		on   bool
	}{
		{[]string{"lodev", "--debug", "exec", "ls"}, []string{"lodev", "exec", "ls"}, true},
		{[]string{"lodev", "exec", "--debug", "ls"}, []string{"lodev", "exec", "--debug", "ls"}, false},
		{[]string{"lodev"}, []string{"lodev"}, false},
	}
	for _, tt := range tests {
		got, on := StripDebugArg(tt.in)
		if !reflect.DeepEqual(got, tt.want) || on != tt.on {
			t.Errorf("StripDebugArg(%v) = %v, %v; want %v, %v", tt.in, got, on, tt.want, tt.on)
		}
	}
}

func TestCommandAliases(t *testing.T) {
	cases := map[string]string{
		"create": "create", "init": "create",
		"remove": "remove", "down": "remove",
		"start": "start", "up": "start",
		"list": "list", "ps": "list",
		"services": "services", "service": "services", "svc": "services",
	}
	for in, want := range cases {
		c, _, err := rootCmd.Find([]string{in})
		if err != nil || c.Name() != want {
			t.Errorf("%s → %v %v, want %s", in, c.Name(), err, want)
		}
	}
	if c, _, err := rootCmd.Find([]string{"delete"}); err == nil && c.Name() == "remove" {
		t.Error("delete must not resolve to remove")
	}
	if c, _, _ := rootCmd.Find([]string{"services", "down"}); c.Parent().Name() != "services" {
		t.Errorf("services down → %s", c.CommandPath())
	}
}
