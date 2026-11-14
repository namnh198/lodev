package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"lodev/internal/config"
)

func writeRepoFile(t *testing.T, p *config.Project, rel, content string) {
	t.Helper()
	f := filepath.Join(p.Root, rel)
	os.MkdirAll(filepath.Dir(f), 0o755)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func trustKeys(t *testing.T, p *config.Project) []string {
	t.Helper()
	items, err := trustItems(p)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, it := range items {
		keys = append(keys, it.Key)
	}
	return keys
}

func TestTrustItemsListsEverything(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Run: "echo hi"}, {Script: "bin/setup.sh"}}})
	p.Cron = true
	p.CronJobs = []string{"* * * * * php artisan schedule:run"}
	p.WebExtraDaemons = []config.Daemon{{Name: "vite", Command: "npm run dev"}}
	writeRepoFile(t, p, "bin/setup.sh", "#!/bin/sh\n")
	writeRepoFile(t, p, ".lodev/hooks/post-start.d/a.sh", "#!/bin/sh\n")
	writeRepoFile(t, p, ".lodev/x.compose.yml", "services: {}\n")
	writeRepoFile(t, p, ".lodev/web-images/Dockerfile", "RUN true\n")
	writeRepoFile(t, p, ".lodev/homeadditional/.gitconfig", "[user]\n")
	writeRepoFile(t, p, ".lodev/homeadditional/.ssh/config", "Host *\n")
	os.Symlink("target", filepath.Join(p.Root, ".lodev/homeadditional/link"))

	got := trustKeys(t, p)
	want := []string{
		"hooks.post-start[0]", "hooks.post-start[1]", "bin/setup.sh",
		".lodev/hooks/post-start.d/a.sh", "cron", "web_extra_daemons.vite", ".lodev/x.compose.yml",
		".lodev/web-images/Dockerfile", ".lodev/homeadditional/.gitconfig",
		".lodev/homeadditional/.ssh/config", ".lodev/homeadditional/link",
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("keys\n got %v\nwant %v", got, want)
	}
}

func TestTrustItemsSkipsExamples(t *testing.T) {
	p := hookProject(t, nil)
	writeRepoFile(t, p, ".lodev/hooks/post-start.d/a.sh.example", "x")
	writeRepoFile(t, p, ".lodev/example.compose.yml.example", "x")
	if got := trustKeys(t, p); len(got) != 0 {
		t.Fatalf("keys = %v, want none", got)
	}
}

func TestTrustItemsMissingScript(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Script: "bin/gone.sh"}}})
	items, err := trustItems(p)
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %v, err = %v", items, err)
	}
	if it := items[0]; it.Key != "bin/gone.sh" || !strings.HasSuffix(it.Display, " (missing)") || it.Sum != sha(nil) {
		t.Fatalf("missing script item = %+v", it)
	}
}

func TestTrustItemsScriptInHookFolderIsOneItem(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Script: ".lodev/hooks/post-start.d/a.sh"}}})
	writeRepoFile(t, p, ".lodev/hooks/post-start.d/a.sh", "x")
	if got := trustKeys(t, p); len(got) != 2 {
		t.Fatalf("keys = %v, want the step and one file", got)
	}
}

func TestTrustItemsFindsHookFolderWithoutConfig(t *testing.T) {
	p := hookProject(t, nil)
	writeRepoFile(t, p, ".lodev/hooks/pre-stop.d/a.sh", "x")
	if got := trustKeys(t, p); !slices.Equal(got, []string{".lodev/hooks/pre-stop.d/a.sh"}) {
		t.Fatalf("keys = %v", got)
	}
}

func TestTrustSumChangesWithContent(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Run: "echo hi"}}})
	writeRepoFile(t, p, ".lodev/hooks/post-start.d/a.sh", "one")
	before, _ := trustItems(p)
	writeRepoFile(t, p, ".lodev/hooks/post-start.d/a.sh", "two")
	after, _ := trustItems(p)
	r := diffTrust(trustSums(before), after)
	if len(r.Changed) != 1 || r.Changed[0].Key != ".lodev/hooks/post-start.d/a.sh" || len(r.Added)+len(r.Removed) != 0 {
		t.Fatalf("report = %+v", r)
	}
}

func TestTrustSumsNonNil(t *testing.T) {
	if trustSums(nil) == nil {
		t.Fatal("trustSums(nil) is nil")
	}
}

func TestDiffTrust(t *testing.T) {
	items := []trustItem{{"a", "a", "1"}, {"b", "b", "3"}, {"c", "c", "4"}}
	r := diffTrust(map[string]string{"a": "1", "b": "2"}, items)
	if len(r.Changed) != 1 || r.Changed[0].Key != "b" || len(r.Added) != 1 || r.Added[0].Key != "c" || len(r.Removed) != 0 {
		t.Fatalf("report = %+v", r)
	}
	r = diffTrust(map[string]string{"a": "1", "b": "2", "c": "4"}, []trustItem{{"a", "a", "1"}})
	if !slices.Equal(r.Removed, []string{"b", "c"}) {
		t.Fatalf("removed = %v", r.Removed)
	}
	if r := diffTrust(trustSums(items), items); !r.Empty() {
		t.Fatalf("equal sets differ: %+v", r)
	}
}

func TestTrustReportString(t *testing.T) {
	r := trustReport{
		Added:   []trustItem{{"hooks.post-start[0]", "hook post-start: echo hi", "1"}, {".lodev/x.compose.yml", "compose: .lodev/x.compose.yml", "1"}},
		Changed: []trustItem{{".lodev/hooks/post-start.d/a.sh", "hook post-start: .lodev/hooks/post-start.d/a.sh", "2"}},
		Removed: []string{"cron"},
	}
	got := r.String()
	for _, want := range []string{
		"hook post-start: echo hi\n",
		".lodev/x.compose.yml (new)",
		".lodev/hooks/post-start.d/a.sh (changed — review with git diff)",
		"removed:\n  cron",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q:\n%s", want, got)
		}
	}
}
