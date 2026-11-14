package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"lodev/internal/config"
	"lodev/internal/registry"
	"lodev/internal/templates"
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

// trustHome points lodev at an empty state dir and registers p with the given approved set.
func trustHome(t *testing.T, p *config.Project, trusted map[string]string) {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg.Put(p)
	e := reg[p.Name]
	e.Trusted = trusted
	reg[p.Name] = e
	if err := reg.Save(); err != nil {
		t.Fatal(err)
	}
}

// approve registers p in an empty state dir with everything its repo runs now already approved.
func approve(t *testing.T, p *config.Project) {
	t.Helper()
	items, err := trustItems(p)
	if err != nil {
		t.Fatal(err)
	}
	trustHome(t, p, trustSums(items))
}

func approvedSet(t *testing.T, name string) map[string]string {
	t.Helper()
	reg, err := registry.Load()
	if err != nil {
		t.Fatal(err)
	}
	return reg[name].Trusted
}

// asTTY makes the gate a terminal (or not) that answers answer.
func asTTY(t *testing.T, tty bool, answer string) {
	t.Helper()
	oldTTY, oldIn := trustTTY, os.Stdin
	t.Cleanup(func() { trustTTY, os.Stdin = oldTTY, oldIn })
	trustTTY = func() bool { return tty }
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString(answer + "\n")
	w.Close()
	os.Stdin = r
}

func hookedProject(t *testing.T) *config.Project {
	return hookProject(t, map[string][]config.HookStep{"post-start": {{Run: "echo hi"}}})
}

func TestEnsureTrustedPreUpgradeRecordsSilently(t *testing.T) {
	p := hookedProject(t)
	trustHome(t, p, nil)
	oldTTY := trustTTY
	t.Cleanup(func() { trustTTY = oldTTY })
	trustTTY = func() bool { t.Fatal("asked"); return true }
	if err := ensureTrusted(p, false, false); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "shop"); got["hooks.post-start[0]"] == "" {
		t.Errorf("not recorded: %v", got)
	}
}

func TestEnsureTrustedEmptyThenHookAsks(t *testing.T) {
	p := hookedProject(t)
	trustHome(t, p, map[string]string{})
	asTTY(t, false, "")
	err := ensureTrusted(p, false, false)
	want := "project shop runs commands from its repo that you have not approved:\n"
	if err == nil || !strings.HasPrefix(err.Error(), want) || !strings.Contains(err.Error(), "hook post-start: echo hi") ||
		!strings.HasSuffix(err.Error(), "run lodev start in a terminal to review them, or pass --yes") {
		t.Fatalf("err = %v", err)
	}
	if len(approvedSet(t, "shop")) != 0 {
		t.Error("recorded without approval")
	}
}

func TestEnsureTrustedNoEntryAsks(t *testing.T) {
	p := hookedProject(t)
	t.Setenv("LODEV_HOME", t.TempDir())
	asTTY(t, false, "")
	if err := ensureTrusted(p, false, false); err == nil {
		t.Fatal("a project the registry does not know must ask")
	}
}

func TestEnsureTrustedYes(t *testing.T) {
	p := hookedProject(t)
	trustHome(t, p, map[string]string{})
	asTTY(t, false, "")
	if err := ensureTrusted(p, true, false); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "shop"); len(got) != 1 {
		t.Errorf("recorded %v", got)
	}
}

func TestEnsureTrustedRemovedKeysDropOut(t *testing.T) {
	p := hookProject(t, nil)
	trustHome(t, p, map[string]string{"cron": "x"})
	asTTY(t, true, "y")
	if err := ensureTrusted(p, false, false); err != nil || len(approvedSet(t, "shop")) != 0 {
		t.Errorf("err %v, set %v", err, approvedSet(t, "shop"))
	}
}

func TestEnsureTrustedPromptDecline(t *testing.T) {
	p := hookedProject(t)
	trustHome(t, p, map[string]string{})
	asTTY(t, true, "n")
	if err := ensureTrusted(p, false, false); err == nil || err.Error() != "not approved: nothing was started" {
		t.Fatalf("err = %v", err)
	}
	if len(approvedSet(t, "shop")) != 0 {
		t.Error("recorded after a no")
	}
}

func TestEnsureTrustedPromptAccept(t *testing.T) {
	p := hookedProject(t)
	trustHome(t, p, map[string]string{})
	asTTY(t, true, "y")
	if err := ensureTrusted(p, false, false); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "shop"); len(got) != 1 {
		t.Errorf("recorded %v", got)
	}
	if err := ensureTrusted(p, false, false); err != nil { // approved: no second question
		t.Errorf("asked again: %v", err)
	}
}

func TestEnsureTrustedFollowsRename(t *testing.T) {
	p := hookedProject(t)
	p.Name = "old"
	approve(t, p)
	p.Name = "new"
	asTTY(t, false, "")
	if err := ensureTrusted(p, false, false); err != nil {
		t.Fatalf("a renamed project must keep its approval: %v", err)
	}
}

func TestEnsureTrustedRenamedPreUpgradeRecordsSilently(t *testing.T) {
	p := hookedProject(t)
	p.Name = "old"
	trustHome(t, p, nil)
	p.Name = "new"
	asTTY(t, false, "")
	if err := ensureTrusted(p, false, false); err != nil {
		t.Fatal(err)
	}
	reg, _ := registry.Load()
	if _, ok := reg["old"]; ok || reg["new"].Trusted["hooks.post-start[0]"] == "" {
		t.Errorf("registry %#v", reg)
	}
}

// start and restart hold the lock; the post-start gate may record, and must not take it again.
func TestRunHooksLockedRecordsWithoutTakingTheLock(t *testing.T) {
	p := hookedProject(t)
	trustHome(t, p, map[string]string{})
	stubHookExec(t, true, nil)
	asTTY(t, true, "y")
	done := make(chan error, 1)
	go func() { done <- withLock(func() error { return runHooks(p, "post-start", true) }) }()
	select {
	case err := <-done:
		if err != nil || len(approvedSet(t, "shop")) != 1 {
			t.Errorf("err %v, set %v", err, approvedSet(t, "shop"))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runHooks took the lock its caller holds")
	}
}

func TestRestartUnapprovedFailsBeforeAnythingRuns(t *testing.T) {
	root := newShop(t)
	writeShop(t, root, shopConf+"hooks:\n  pre-stop: [\"echo down\"]\n")
	p, err := currentProject()
	if err != nil {
		t.Fatal(err)
	}
	trustHome(t, p, map[string]string{})
	ran := stubHookExec(t, true, nil)
	asTTY(t, false, "")
	oldRun, oldStart := dockerRun, startFn
	t.Cleanup(func() { dockerRun, startFn = oldRun, oldStart })
	var calls []string
	dockerRun = func(a ...string) error { calls = append(calls, strings.Join(a, " ")); return nil }
	startFn = func(*config.Project, startOpts) error { calls = append(calls, "start"); return nil }
	if err := restartProject(p, startOpts{}); err == nil || !strings.Contains(err.Error(), "have not approved") {
		t.Fatalf("err = %v", err)
	}
	if len(calls) != 0 || len(*ran) != 0 {
		t.Errorf("docker %q, hooks %q", calls, *ran)
	}
}

func TestStartChecksTrustFirst(t *testing.T) {
	p := hookedProject(t)
	trustHome(t, p, map[string]string{})
	asTTY(t, false, "")
	oldUp := servicesUpFn
	t.Cleanup(func() { servicesUpFn = oldUp })
	called := false
	servicesUpFn = func() error { called = true; return nil }
	if err := start(p, startOpts{}); err == nil || !strings.Contains(err.Error(), "have not approved") {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Error("services started before the gate")
	}
}

func TestRunHooksRefusesChangedHook(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"pre-stop": {{Script: ".lodev/hooks/down.sh"}}})
	writeHookFile(t, p, ".lodev/hooks/down.sh", 0o755)
	items, _ := trustItems(p)
	trustHome(t, p, trustSums(items))
	ran := stubHookExec(t, true, nil)
	asTTY(t, false, "")
	writeRepoFile(t, p, ".lodev/hooks/down.sh", "#!/bin/sh\nrm -rf /\n") // as after a git pull
	if err := runHooks(p, "pre-stop", false); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("err = %v", err)
	}
	if len(*ran) != 0 {
		t.Errorf("ran %q", *ran)
	}
}

func TestStopHintsNoHooksWhenNotApproved(t *testing.T) {
	root := newShop(t)
	writeShop(t, root, shopConf+"hooks:\n  pre-stop: [\"echo down\"]\n")
	p, err := currentProject()
	if err != nil {
		t.Fatal(err)
	}
	trustHome(t, p, map[string]string{})
	stubHookExec(t, true, nil)
	asTTY(t, false, "")
	// --no-hooks=false: TestStopRemovesRoutes leaves the flag set.
	if err := runRoot(t, "stop", "--no-hooks=false"); err == nil || !strings.Contains(err.Error(), "lodev stop --no-hooks stops anyway") {
		t.Errorf("err = %v", err)
	}
}

func TestCreateExistingAsks(t *testing.T) {
	root := newShop(t)
	writeShop(t, root, shopConf+"hooks:\n  post-start: [\"echo hi\"]\n")
	asTTY(t, false, "")
	if err := runRoot(t, "create"); err == nil || !strings.Contains(err.Error(), "have not approved") {
		t.Fatalf("err = %v", err)
	}
	if reg, _ := registry.Load(); len(reg) != 0 {
		t.Fatalf("registered anyway: %v", reg)
	}
	if err := runRoot(t, "create", "--yes"); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "shop"); got["hooks.post-start[0]"] == "" {
		t.Errorf("sums not recorded: %v", got)
	}
}

func TestCreateExistingNothingToRunRecordsEmpty(t *testing.T) {
	newShop(t)
	asTTY(t, false, "")
	if err := runRoot(t, "create"); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "shop"); got == nil || len(got) != 0 {
		t.Errorf("want {}, got %#v", got)
	}
}

func TestCreateFreshRecordsEmpty(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := runRoot(t, "create", "--no-ask", "--name", "fresh"); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "fresh"); got == nil {
		t.Error("a fresh project must be recorded as approved, not pre-upgrade")
	}
}

// start mounts what a symlinked .lodev/homeadditional points to: its files are listed and hashed.
func TestTrustItemsFollowsSymlinkedHomeAdditional(t *testing.T) {
	p := hookProject(t, nil)
	target := filepath.Join(t.TempDir(), "dotfiles")
	os.MkdirAll(target, 0o755)
	os.WriteFile(filepath.Join(target, ".gitconfig"), []byte("one"), 0o644)
	os.MkdirAll(filepath.Join(p.Root, ".lodev"), 0o755)
	if err := os.Symlink(target, filepath.Join(p.Root, ".lodev", "homeadditional")); err != nil {
		t.Fatal(err)
	}
	before, err := trustItems(p)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(target, ".gitconfig"), []byte("two"), 0o644)
	after, _ := trustItems(p)
	r := diffTrust(trustSums(before), after)
	if len(r.Changed) != 1 || r.Changed[0].Key != ".lodev/homeadditional/.gitconfig" || len(r.Added)+len(r.Removed) != 0 {
		t.Fatalf("report = %+v", r)
	}
}

func TestTrustItemsEnvironment(t *testing.T) {
	p := hookProject(t, nil)
	p.Environment = map[string]string{"B": "2", "A": "1"}
	items, err := trustItems(p)
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %v, err = %v", items, err)
	}
	if it := items[0]; it.Key != "environment" || it.Display != "environment: A=1\nenvironment: B=2" || it.Sum != sha([]byte("A=1\nB=2\n")) {
		t.Fatalf("item = %+v", it)
	}
	if !isConfigKey("environment") {
		t.Error("environment is config, not a file")
	}
}

func TestTrustItemsApacheConfig(t *testing.T) {
	p := hookProject(t, nil)
	writeRepoFile(t, p, ".lodev/apache/example.conf.example", "x")
	writeRepoFile(t, p, ".lodev/apache-webserver.conf", templates.GeneratedMarker+"\n")
	if got := trustKeys(t, p); len(got) != 0 {
		t.Fatalf("examples and the generated conf must not be listed: %v", got)
	}
	writeRepoFile(t, p, ".lodev/apache/log.conf", `CustomLog "|/bin/sh -c x" common`)
	writeRepoFile(t, p, ".lodev/apache/sub/x.conf", "x")
	writeRepoFile(t, p, ".lodev/apache-webserver.conf", "<VirtualHost *:80>\n")
	want := []string{".lodev/apache-webserver.conf", ".lodev/apache/log.conf", ".lodev/apache/sub/x.conf"}
	if got := trustKeys(t, p); !slices.Equal(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}

func TestTrustItemsSkipsFileNamedLikeHookFolder(t *testing.T) {
	p := hookProject(t, map[string][]config.HookStep{"post-start": {{Run: "echo hi"}}})
	writeRepoFile(t, p, ".lodev/hooks/pre-stop.d", "not a folder")
	if got := trustKeys(t, p); !slices.Equal(got, []string{"hooks.post-start[0]"}) {
		t.Fatalf("keys = %v", got)
	}
}

// writeFreshHook puts a hook script in the current folder, which has no .lodev/config.yaml yet.
func writeFreshHook(t *testing.T) {
	t.Helper()
	t.Setenv("LODEV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	os.MkdirAll(".lodev/hooks/post-start.d", 0o755)
	if err := os.WriteFile(".lodev/hooks/post-start.d/a.sh", []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	asTTY(t, false, "")
}

func TestCreateFreshAsksForRepoCommands(t *testing.T) {
	writeFreshHook(t)
	if err := runRoot(t, "create", "--no-ask", "--name", "fresh"); err == nil || !strings.Contains(err.Error(), "have not approved") {
		t.Fatalf("err = %v", err)
	}
	if reg, _ := registry.Load(); len(reg) != 0 {
		t.Fatalf("registered anyway: %v", reg)
	}
}

func TestCreateFreshYesRecords(t *testing.T) {
	writeFreshHook(t)
	if err := runRoot(t, "create", "--no-ask", "--name", "fresh", "--yes"); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "fresh"); got[".lodev/hooks/post-start.d/a.sh"] == "" {
		t.Errorf("sums not recorded: %v", got)
	}
}

// A fresh apache project has only lodev's scaffolded examples: nothing to ask about.
func TestCreateFreshApacheNothingToAsk(t *testing.T) {
	t.Setenv("LODEV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	asTTY(t, false, "")
	if err := runRoot(t, "create", "--no-ask", "--name", "fresh", "--webserver", "apache"); err != nil {
		t.Fatal(err)
	}
	if got := approvedSet(t, "fresh"); got == nil || len(got) != 0 {
		t.Errorf("want {}, got %#v", got)
	}
}
