package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
	if it := items[0]; it.Key != "bin/gone.sh" || !strings.HasSuffix(it.Display, " (missing)") {
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

func TestCreateExistingAsks(t *testing.T) {
	root := newShop(t)
	writeShop(t, root, shopConf+"hooks:\n  post-start: [\"echo hi\"]\n")
	asTTY(t, false, "")
	if err := runRoot(t, "create"); err == nil || !strings.Contains(err.Error(), "run lodev create in a terminal") {
		t.Fatalf("err = %v", err)
	}
	if reg, _ := registry.Load(); len(reg) != 0 {
		t.Fatalf("registered anyway: %v", reg)
	}
	if err := runRoot(t, "create", "--yes"); err != nil {
		t.Fatal(err)
	}
	if reg, _ := registry.Load(); reg["shop"].Path == "" {
		t.Errorf("not registered: %v", reg)
	}
}

// start copies what a symlinked .lodev/homeadditional points to: its files are listed.
func TestTrustItemsFollowsSymlinkedHomeAdditional(t *testing.T) {
	p := hookProject(t, nil)
	target := filepath.Join(t.TempDir(), "dotfiles")
	os.MkdirAll(target, 0o755)
	os.WriteFile(filepath.Join(target, ".gitconfig"), []byte("one"), 0o644)
	os.MkdirAll(filepath.Join(p.Root, ".lodev"), 0o755)
	if err := os.Symlink(target, filepath.Join(p.Root, ".lodev", "homeadditional")); err != nil {
		t.Fatal(err)
	}
	if got := trustKeys(t, p); !slices.Equal(got, []string{".lodev/homeadditional/.gitconfig"}) {
		t.Fatalf("keys = %v", got)
	}
}

func TestTrustItemsEnvironment(t *testing.T) {
	p := hookProject(t, nil)
	p.Environment = map[string]string{"B": "2", "A": "1"}
	items, err := trustItems(p)
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %v, err = %v", items, err)
	}
	if it := items[0]; it.Key != "environment" || it.Display != "environment: A=1\nenvironment: B=2" {
		t.Fatalf("item = %+v", it)
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
	if err := runRoot(t, "create", "--no-ask", "--name", "fresh"); err == nil || !strings.Contains(err.Error(), "run lodev create in a terminal") {
		t.Fatalf("err = %v", err)
	}
	if reg, _ := registry.Load(); len(reg) != 0 {
		t.Fatalf("registered anyway: %v", reg)
	}
}

func TestConfirmRepoCommands(t *testing.T) {
	empty := hookProject(t, nil)
	asTTY(t, false, "")
	if err := confirmRepoCommands(empty, false); err != nil {
		t.Fatalf("nothing to run: %v", err)
	}
	p := hookedProject(t)
	want := "project shop runs commands from its repo:\n  hook post-start: echo hi\nrun lodev create in a terminal to review them, or pass --yes"
	if err := confirmRepoCommands(p, false); err == nil || err.Error() != want {
		t.Fatalf("non-TTY err = %v", err)
	}
	if err := confirmRepoCommands(p, true); err != nil {
		t.Fatalf("--yes: %v", err)
	}
	asTTY(t, true, "n")
	if err := confirmRepoCommands(p, false); err == nil || err.Error() != "not approved: nothing was registered" {
		t.Fatalf("decline err = %v", err)
	}
	asTTY(t, true, "y")
	if err := confirmRepoCommands(p, false); err != nil {
		t.Fatalf("accept: %v", err)
	}
}

// After create, start and hooks run the repo's commands without asking (the check is create-only).
func TestStartDoesNotAsk(t *testing.T) {
	p, _ := startStubs(t, "services:\n  web:\n    networks: {lodev: {}, default: {}}\n")
	writeRepoFile(t, p, ".lodev/hooks/post-start.d/a.sh", "#!/bin/sh\n")
	os.Chmod(filepath.Join(p.Root, ".lodev/hooks/post-start.d/a.sh"), 0o755)
	asTTY(t, false, "")
	got := stubHookExec(t, true, nil)
	if err := start(p, startOpts{}); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || !strings.Contains(strings.Join((*got)[0], " "), "a.sh") {
		t.Fatalf("hook runs = %v", *got)
	}
}

// The scaffolded .bash_aliases.example is not copied into the home, so it is not listed either.
func TestTrustItemsSkipsHomeAdditionalExamples(t *testing.T) {
	p := hookProject(t, nil)
	writeRepoFile(t, p, ".lodev/homeadditional/.bash_aliases.example", "# x")
	writeRepoFile(t, p, ".lodev/homeadditional/.bash_aliases", "alias x=y")
	if got := trustKeys(t, p); !slices.Equal(got, []string{".lodev/homeadditional/.bash_aliases"}) {
		t.Fatalf("keys = %v", got)
	}
}
