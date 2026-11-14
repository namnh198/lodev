# lodev

Go CLI (cobra, yaml.v3) that runs local PHP sites in Docker: a shared stack (traefik, mysql,
redis, mailpit...) plus one `lodev-<name>-web` container per project. `README.md` is the user
manual and the spec of record for behaviour; `make help` lists build targets.

## Verify

- `go vet ./... && go test ./...` is the gate (CI runs the same plus `make packages`).
- Run Go commands outside the sandbox with `GOCACHE=$TMPDIR/gocache`: inside, Go cannot reach
  its module cache or verify TLS, and `httptest` cannot bind a port.
- `gofmt -l cmd internal tools assets` prints nothing before a commit.
- Golden files: `go test ./internal/templates/ ./internal/config/ -update`, then read the diff.

## Change discipline

- TDD: a test that fails for the right reason first, then the code. Tests stub the outside
  world through package vars (`dockerRun`, `containerRunning`, `userReady`, `execInWeb` via
  `stubHookExec`, `startStubs`) and isolate state with `t.Setenv("LODEV_HOME", t.TempDir())`.
- A behaviour change updates `README.md` in the same commit (its section and the commands table).
- Commits: conventional prefix (`feat:`, `fix:`, `refactor:`, `chore:`), lowercase, the
  subject says what the user gets. One commit per logical change; stage files by name.
- Plans and specs live in `docs/superpowers/` and `.superpowers/` (git-ignored); never commit them.

## Real Docker checks (e2e)

`LODEV_HOME` isolates files, never container names, ports or `/etc/hosts`.

1. `docker ps` first; pick a scratch project name no container uses.
2. Back up `~/.lodev/projects.json`, `~/.lodev/config.yaml`, `/etc/hosts` to the scratchpad.
3. Build the branch binary into the scratchpad and run it from a scratch project folder.
4. Clean up: `lodev remove --yes --volumes`, delete the scratch data folder, restore the
   backups, run `~/.local/bin/lodev service start` (the installed lodev), then `cmp` each backup.

Done means every backup compares equal and the user's real sites answer as before.

## Topic rules

`.claude/rules/` holds rules scoped to the files they govern: Go code, the web container,
repo-supplied commands, and releases.
