package config

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHookStepYAML(t *testing.T) {
	in := "post-start:\n    - composer install\n    - script: .lodev/hooks/a.sh\n"
	var h map[string][]HookStep
	if err := yaml.Unmarshal([]byte(in), &h); err != nil {
		t.Fatal(err)
	}
	want := map[string][]HookStep{"post-start": {{Run: "composer install"}, {Script: ".lodev/hooks/a.sh"}}}
	if !reflect.DeepEqual(h, want) {
		t.Fatalf("got %+v", h)
	}
	out, err := yaml.Marshal(h)
	if err != nil || string(out) != in {
		t.Errorf("round trip %q, %v", out, err)
	}
}

func TestValidateHooks(t *testing.T) {
	base := Project{Name: "x", Type: "php", PHP: "8.4", Webserver: "nginx"}
	parse := func(s string) map[string][]HookStep {
		var h map[string][]HookStep
		if err := yaml.Unmarshal([]byte(s), &h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	for _, c := range []struct{ hooks, want string }{
		{"post-build: [x]", `hooks: unknown event "post-build" (allowed: post-start, pre-stop, post-db-import, post-snapshot-restore)`},
		{`post-start: [""]`, "hooks.post-start[0] is empty"},
		{"post-start: [{run: x}]", `hooks.post-start[0]: only "script" is allowed`},
		{"post-start: [ok, {script: ../x.sh}]", `hooks.post-start[1]: script "../x.sh" must be a file inside the project`},
		{"pre-stop: [{script: /abs.sh}]", `hooks.pre-stop[0]: script "/abs.sh" must be a file inside the project`},
		{"post-start: [composer install, {script: .lodev/hooks/a.sh}]", ""},
	} {
		p := base
		p.Hooks = parse(c.hooks)
		err := p.Validate()
		if c.want == "" && err != nil || c.want != "" && (err == nil || err.Error() != c.want) {
			t.Errorf("%s: err = %v, want %q", c.hooks, err, c.want)
		}
	}
	if !strings.Contains(strings.Join(HookEvents, ","), "post-snapshot-restore") {
		t.Error("HookEvents")
	}
}
