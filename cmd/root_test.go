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
