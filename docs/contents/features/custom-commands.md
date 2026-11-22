---
title: "Custom commands"
description: "Add your own lodev commands for the host or the web container, with autocomplete."
---

`lodev <name> [args...]` runs a script file found in one of these folders:

| Scope | Runs in | Folder |
|---|---|---|
| global | the project's web container | `~/.lodev/commands/web/<name>` |
| global | your machine | `~/.lodev/commands/host/<name>` |
| project | the project's web container | `<project>/.lodev/commands/web/<name>` |
| project | your machine | `<project>/.lodev/commands/host/<name>` |

The file name is the command name: `a-z`, `0-9` and `-`, starting with a letter or digit. Dot
files, folders and `*.example` files are skipped; any other bad name, or an unknown `Type`, is
skipped with a warning. `lodev create` scaffolds `.lodev/commands/{web,host}/hello.example`:
rename one to `hello` to try it.

The leading `#` lines are the header. lodev reads three keys:

```bash
# Usage: hello [args...]
# Description: Say hello in the web container
# Type: laravel, php
echo "hello $*"
```

- `Usage` is the help line; its first word must be the file name (else `<name> [args...]`).
- `Description` is the help text (else `Custom command (<path>)`).
- `Type`: a comma-separated list of project types. Inside a project, the command exists only
  in projects of those types; outside one, it is not filtered. Without it, the command is for
  every project.

A file with no shebang, or a bash one (`#!/bin/bash`, `#!/usr/bin/env bash`), runs with
`bash -c`; `$0` is the command name. A file whose first line is another shebang
(`#!/usr/bin/env php`, `#!/usr/bin/python3`) runs with that interpreter, which must exist where the
command runs (the web container, or your machine): lodev writes the file to a temporary executable
and runs it, so `$0` is that temp file's path, and the file is removed afterwards. Either way
`"$@"` is the args, passed unchanged (lodev parses no flags after the name) and the exit code is
the script's. Options on a bash shebang (`#!/bin/bash -e`) are ignored, because the file runs with
`bash -c`: use `set -e` in the script instead.

The header lines are read after a shebang line too, but the interpreter still reads them as code, so
they must be comments for it. That holds for sh, bash, python, ruby and perl. It breaks PHP (the
`#` lines before `<?php` are printed on every run, since PHP strips only line 1) and Node (a `#`
line after line 1 is a syntax error). A PHP or Node command therefore gets no header (Use and
Description take their defaults, and there is no Type filter), or is wrapped in a small bash
command that calls the interpreter, e.g. `.lodev/commands/web/report`:

```bash
# Type: magento
exec php bin/report.php "$@"
```

**Precedence:** project beats global, whatever the folder. Within one scope, the same name in
`web/` and `host/` warns and the web one wins. A file whose name is a lodev command or alias
(`start`, `up`, `help`…) is skipped with a warning; it never replaces lodev's own. A file in
`~/.lodev/commands/` whose marker line is on line 1, or on line 2 when line 1 is a shebang, is
lodev's own copy, not yours.
Type filtering comes first, so a project `magento` limited to `magento` does not hide a global one
in a laravel project.

**Project commands need a registered project** (one made by `lodev create`, which lists them for
review, see [Commands from the project's repo](repo-commands.md)). In a cloned
folder that is not registered, `.lodev/commands/` is ignored, with the hint
`.lodev/commands is not loaded: run lodev create to review the project's commands`. When the
project's `config.yaml` does not load, the project's commands are not loaded and the type filter is
off: the shipped and global commands all exist (`artisan` and `wp` too). A web one reports the
config error when you run it; a host one runs as if outside a project, in the current folder, and
does not report it.

**Shipped commands.** lodev ships `php`, `composer`, `npm`, `node`, `pnpm`, `yarn`, `grunt`,
`artisan`, `wp`, `magento` and `n98` as web command files, written to `~/.lodev/commands/web/`
by any command except `help`, `version`, `completion` and `self-upgrade` (when missing, or
when the file still has the marker line, on line 1 or on line 2 after a shebang, and differs).
The host commands below are synced the same way into `~/.lodev/commands/host/`. Delete the marker line to edit one, web or host: lodev
then leaves the file alone and runs it instead of its own copy. A file lodev cannot read, or a
symlink, is left as it is too. A command of the same name in
the project or elsewhere in `~/.lodev/commands/` also replaces the shipped one. `artisan` exists
only in laravel projects, `wp` only in wordpress ones, and `magento` and `n98` only in magento
ones.

lodev also ships these host commands, which open the project's MySQL/MariaDB database in a
GUI. Each needs the database published on a host port (`lodev config-global --mysql-port=3306`
or `--mariadb-port=3307`, then `lodev service start`) and exits 1 with the reason when there
is no project, no database service, no published port, or the OS has no such app. The
connection is named `lodev-<project>`. On Linux they also exit 1 when the app's command is
not on PATH. `dbeaver` and `mysqlworkbench` pass their arguments through `open -a ... --args`,
which macOS honors only when the app is not already running: quit it first.

| Command | App | Notes |
|---|---|---|
| `tableplus` | TablePlus | `mysql://` or `mariadb://` URL; `xdg-open` on Linux |
| `sequelace` | Sequel Ace | macOS only; `mysql://` URL, also for MariaDB |
| `dbeaver` | DBeaver | `-con` connection string; `dbeaver` on Linux |
| `mysqlworkbench` | MySQL Workbench | `--query user@host:port`; it asks for the password (`root`); `mysql-workbench` on Linux |
| `tablepro` | TablePro | macOS only; `mysql://` or `mariadb://` URL; TablePro asks you to confirm the connection |

`vscode` opens VS Code attached to the project's web container. It needs `code` on PATH and the
Dev Containers extension, and runs `lodev start` first when the project is stopped. It removes the
container user's `~/.gitconfig` so the extension copies yours, then sets `core.autocrlf input` and
`safe.directory '*'` again. Because the web image has no `USER`, it writes VS Code's per-container
config (`nameConfigs/lodev-<project>-web.json` under `~/Library/Application Support/Code/User` on
macOS, `~/.config/Code/User` on Linux) with the project user and `working_dir`, only when the file
is missing, so your edits are kept. Ported from
[tyler36/ddev-vscode-devcontainer](https://github.com/tyler36/ddev-vscode-devcontainer) (Apache-2.0).

**Web commands** run like `lodev exec`: as the project user, in `working_dir`, with a TTY when
you have one, and their exit code. They need a running project.

**Host commands** run in the project root (outside a project: the current folder), with your
environment plus:

| Variable | Value |
|---|---|
| `LODEV_BIN` | the lodev binary running the command: call `"$LODEV_BIN"`, not `lodev` from PATH |
| `LODEV_PWD` | the folder lodev was run from |
| `LODEV_PROJECT`, `LODEV_ROOT`, `LODEV_TYPE` | the project's name, root folder and type |
| `LODEV_URL` | `https://<first domain>`, empty for `webserver: none` |
| `LODEV_WEB_CONTAINER`, `LODEV_CONTAINER_USER`, `LODEV_WORKDIR` | the web container, its project user and `working_dir` |
| `LODEV_PROJECT_STATUS` | `running` or `stopped` |
| `LODEV_DB_TYPE`, `LODEV_DB_HOST`, `LODEV_DB_PORT`, `LODEV_DB_USER`, `LODEV_DB_PASSWORD`, `LODEV_DB_NAME` | with MySQL or MariaDB: `mysql`/`mariadb`, `127.0.0.1`, the global `ports.mysql`/`ports.mariadb` (empty when not published), `root`, `root`, the project's database |

Only `LODEV_BIN` and `LODEV_PWD` are set outside a project. Any of these variables inherited from
a parent lodev command is removed first, so a nested command sees only its own values (other
`LODEV_*` such as `LODEV_HOME` and `LODEV_DEBUG` pass through). Host commands without a non-bash
shebang need `bash` on PATH, but no Docker and no running project.

## Autocomplete scripts

A command can complete its arguments on TAB through an autocomplete script: a file named like the
command in an `autocomplete/` folder next to it, e.g. `.lodev/commands/web/autocomplete/report`
for `.lodev/commands/web/report`.

- Its arguments are the words typed after `lodev <name>`; the last one is the word being completed,
  and may be `""`.
- It prints one suggestion per line, `value` or `value<TAB>description`. lodev keeps the lines whose
  value starts with the word being completed.
- It runs like its command: the same shebang rule, web scripts in the web container (as the
  project user, in `working_dir`, without a TTY), host scripts on your machine with the variables
  above. Its stderr is discarded.
- It may run 5 seconds. No suggestion, a failure, a timeout, or a web script outside a project or
  with the project stopped lets the shell complete file names instead.

The script comes from the same scope as the command that won: a shipped command uses lodev's own
autocomplete script, a `~/.lodev/commands/` command the one in `~/.lodev/commands/<kind>/autocomplete/`,
and a project command the project's. There is no fallback across scopes: a file that replaces a
shipped command has no completion until you add its autocomplete script too. lodev's own scripts are
synced to `~/.lodev/commands/<kind>/autocomplete/` like the commands, with the same marker rule; a
synced copy that still has the marker is never used for your own command of that name. Removing the
marker from a synced `~/.lodev/commands/<kind>/autocomplete/<name>` has no effect while the command
file itself still carries lodev's marker: the shipped command and its embedded script win. To
customise a shipped command's completion, take over both files. A project's
autocomplete scripts load only for a registered project, and `lodev create` lists each one as
`autocomplete for lodev <name> (web container): .lodev/commands/web/autocomplete/<name>` (or
`(runs on this machine)` for host). Warnings about custom command files are not printed while the
shell completes.

**Shipped autocomplete scripts.** `composer`, `artisan` and `magento` complete their commands, options
and (composer) packages through the tool's own Symfony `_complete`; `wp` through `wp cli completions`.
`n98` has none: its `_complete` leaves out n98's own commands.
