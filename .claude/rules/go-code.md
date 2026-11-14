---
paths:
  - "**/*.go"
---

# Go code

- Errors are lowercase, plain sentences that name the thing and the next step the user can
  take (`run lodev create in a terminal to review them, or pass --yes`). Tests compare them
  exactly, so a wording change updates its test.
- User-facing output goes through `internal/ui` (`ui.Info`, `ui.Warn`, `ui.Success`); tests read
  it with `ui.Capture`.
- Comments: one line on the why the code cannot show.
- Code that writes into a folder a running container bind-mounts replaces the folder's
  contents in place (`snapshotTree`, `os.WriteFile` truncation): a new inode leaves the container
  on a deleted directory.
- Paths under `~/.lodev` come from helpers (`config.Home()`, `g.DataPath()`, `projectHome`),
  never string concatenation in callers.
- Platform code is split by build tag (`lock_unix.go` / `lock_windows.go`); `make build` cross-
  compiles windows/darwin/linux, so a unix-only call outside those files breaks the release.
- Test helpers for a project in a temp dir already exist (`newShop`, `writeShop`, `hookProject`,
  `writeRepoFile`, `writeFile`, `mustContain`, `runRoot`, `runConfig`): reuse them.
