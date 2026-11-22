---
title: "Custom commands"
description: "Add your own lodev commands for the host or the web container, with autocomplete."
---

Add your own `lodev <name>` commands, run in the web container or on your machine.

`lodev <name> [args...]` runs a script found in one of these folders:

| Scope | Runs in | Folder |
|---|---|---|
| global | the project's web container | `~/.lodev/commands/web/<name>` |
| global | your machine | `~/.lodev/commands/host/<name>` |
| project | the project's web container | `<project>/.lodev/commands/web/<name>` |
| project | your machine | `<project>/.lodev/commands/host/<name>` |

## Naming and header

- The file name is the command name: `a-z`, `0-9` and `-`, starting with a letter or digit.
- Skipped: dot files, folders and `*.example` files.
- Skipped with a warning: any other bad name, or an unknown `Type`.
- `lodev create` scaffolds `.lodev/commands/{web,host}/hello.example`. Rename one to `hello` to try it.

The leading `#` lines are the header. lodev reads three keys:

```bash
# Usage: hello [args...]
# Description: Say hello in the web container
# Type: laravel, php
echo "hello $*"
```

- `Usage`: the help line. Its first word must be the file name, else `<name> [args...]`.
- `Description`: the help text, else `Custom command (<path>)`.
- `Type`: a comma-separated list of project types.
  - Inside a project, the command exists only in those types.
  - Outside a project, it is not filtered.
  - Without `Type`, the command is for every project.

## How a file runs

- No shebang, or a bash one (`#!/bin/bash`, `#!/usr/bin/env bash`): runs with `bash -c`. `$0` is the command name.
- Any other shebang (`#!/usr/bin/env php`, `#!/usr/bin/python3`): runs with that interpreter.
  - The interpreter must exist where the command runs (the web container, or your machine).
  - lodev writes the file to a temporary executable and runs it. `$0` is that temp path, and the file is removed afterwards.
- `"$@"` is the args, passed unchanged. lodev parses no flags after the name.
- The exit code is the script's.
- Options on a bash shebang (`#!/bin/bash -e`) are ignored, because the file runs with `bash -c`. Use `set -e` in the script instead.

The header lines are read after a shebang line too, so they must be comments for the interpreter.
That holds for sh, bash, python, ruby and perl. It breaks two languages:

- PHP: the `#` lines before `<?php` are printed on every run, since PHP strips only line 1.
- Node: a `#` line after line 1 is a syntax error.

So a PHP or Node command gets no header (Use and Description take their defaults, and there is no
Type filter). Or wrap it in a small bash command that calls the interpreter, e.g. `.lodev/commands/web/report`:

```bash
# Type: magento
exec php bin/report.php "$@"
```

## Precedence

- A project command beats a global one, whatever the folder.
- Within one scope, the same name in `web/` and `host/` warns, and the web one wins.
- A file named like a lodev command or alias (`start`, `up`, `help`...) is skipped with a warning. It never replaces lodev's own.
- A file in `~/.lodev/commands/` whose marker line is on line 1 (or line 2 when line 1 is a shebang) is lodev's own copy, not yours.
- Type filtering runs first. A project `magento` command limited to `magento` does not hide a global one in a laravel project.

## Registered projects only

- Project commands need a registered project: one made by `lodev create`, which lists them for review (see [Commands from the project's repo](repo-commands.md)).
- In a cloned folder that is not registered, `.lodev/commands/` is ignored. The hint is `.lodev/commands is not loaded: run lodev create to review the project's commands`.
- When the project's `config.yaml` does not load:
  - Project commands are not loaded, and the type filter is off.
  - Shipped and global commands all exist (`artisan` and `wp` too).
  - A web command reports the config error when you run it.
  - A host command runs as if outside a project, in the current folder, and does not report it.

## Shipped commands

- lodev ships these web command files: `php`, `composer`, `npm`, `node`, `pnpm`, `yarn`, `grunt`, `artisan`, `wp`, `magento`, `n98`.
- They are written to `~/.lodev/commands/web/` by any command except `help`, `version`, `completion` and `self-upgrade`.
- A file is rewritten only when it is missing, or when it still has the marker line (on line 1, or on line 2 after a shebang) and differs.
- The host commands below are synced the same way into `~/.lodev/commands/host/`.
- To edit one, delete its marker line. lodev then leaves the file alone and runs it instead of its own copy.
- A file lodev cannot read, or a symlink, is left as it is too.
- A command of the same name in the project, or elsewhere in `~/.lodev/commands/`, also replaces the shipped one.
- Availability by project type:
  - `artisan`: laravel only.
  - `wp`: wordpress only.
  - `magento` and `n98`: magento only.

### Database GUI commands

lodev also ships host commands that open the project's MySQL/MariaDB database in a GUI.

- Each needs the database published on a host port: `lodev config-global --mysql-port=3306` or `--mariadb-port=3307`, then `lodev service start`.
- Each exits 1 with the reason when there is no project, no database service, no published port, or no such app on the OS.
- The connection is named `lodev-<project>`.
- On Linux, they also exit 1 when the app's command is not on PATH.
- `dbeaver` and `mysqlworkbench` pass their arguments through `open -a ... --args`. macOS honors that only when the app is not already running, so quit it first.

| Command | App | Notes |
|---|---|---|
| `tableplus` | TablePlus | `mysql://` or `mariadb://` URL; `xdg-open` on Linux |
| `sequelace` | Sequel Ace | macOS only; `mysql://` URL, also for MariaDB |
| `dbeaver` | DBeaver | `-con` connection string; `dbeaver` on Linux |
| `mysqlworkbench` | MySQL Workbench | `--query user@host:port`; it asks for the password (`root`); `mysql-workbench` on Linux |
| `tablepro` | TablePro | macOS only; `mysql://` or `mariadb://` URL; TablePro asks you to confirm the connection |

### vscode

`vscode` opens VS Code attached to the project's web container.

- Needs `code` on PATH and the Dev Containers extension.
- Runs `lodev start` first when the project is stopped.
- Removes the container user's `~/.gitconfig`, so the extension copies yours. Then it sets `core.autocrlf input` and `safe.directory '*'` again.
- The web image has no `USER`, so it writes VS Code's per-container config with the project user and `working_dir`:
  - `nameConfigs/lodev-<project>-web.json` under `~/Library/Application Support/Code/User` on macOS, or `~/.config/Code/User` on Linux.
  - Only when the file is missing, so your edits are kept.
- Ported from [tyler36/ddev-vscode-devcontainer](https://github.com/tyler36/ddev-vscode-devcontainer) (Apache-2.0).

## Running a command

**Web commands**

- Run like `lodev exec`: as the project user, in `working_dir`, with a TTY when you have one.
- They return the script's exit code.
- They need a running project.

**Host commands**

- Run in the project root. Outside a project, they run in the current folder.
- They get your environment plus these variables:

| Variable | Value |
|---|---|
| `LODEV_BIN` | the lodev binary running the command: call `"$LODEV_BIN"`, not `lodev` from PATH |
| `LODEV_PWD` | the folder lodev was run from |
| `LODEV_PROJECT`, `LODEV_ROOT`, `LODEV_TYPE` | the project's name, root folder and type |
| `LODEV_URL` | `https://<first domain>`, empty for `webserver: none` |
| `LODEV_WEB_CONTAINER`, `LODEV_CONTAINER_USER`, `LODEV_WORKDIR` | the web container, its project user and `working_dir` |
| `LODEV_PROJECT_STATUS` | `running` or `stopped` |
| `LODEV_DB_TYPE`, `LODEV_DB_HOST`, `LODEV_DB_PORT`, `LODEV_DB_USER`, `LODEV_DB_PASSWORD`, `LODEV_DB_NAME` | with MySQL or MariaDB: `mysql`/`mariadb`, `127.0.0.1`, the global `ports.mysql`/`ports.mariadb` (empty when not published), `root`, `root`, the project's database |

- Outside a project, only `LODEV_BIN` and `LODEV_PWD` are set.
- Any of these variables inherited from a parent lodev command is removed first, so a nested command sees only its own values.
- Other `LODEV_*` variables, such as `LODEV_HOME` and `LODEV_DEBUG`, pass through.
- Host commands without a non-bash shebang need `bash` on PATH. They need no Docker and no running project.

## Autocomplete scripts

A command can complete its arguments on TAB with an autocomplete script. It is a file named like the
command, in an `autocomplete/` folder next to it. For example, `.lodev/commands/web/autocomplete/report`
completes `.lodev/commands/web/report`.

- Arguments: the words typed after `lodev <name>`. The last one is the word being completed, and may be `""`.
- Output: one suggestion per line, as `value` or `value<TAB>description`. lodev keeps the lines whose value starts with the word being completed.
- It runs like its command:
  - Same shebang rule.
  - Web scripts run in the web container, as the project user, in `working_dir`, without a TTY.
  - Host scripts run on your machine with the variables above.
  - Its stderr is discarded.
- Time limit: 5 seconds.
- The shell falls back to file names when the script gives no suggestion, fails, times out, or is a web script run outside a project or with the project stopped.
- Scope: the script comes from the same scope as the command that won.
  - A shipped command uses lodev's own autocomplete script.
  - A `~/.lodev/commands/` command uses `~/.lodev/commands/<kind>/autocomplete/`.
  - A project command uses the project's.
- No fallback across scopes. A file that replaces a shipped command has no completion until you add its autocomplete script too.
- lodev's own scripts are synced to `~/.lodev/commands/<kind>/autocomplete/` with the same marker rule. A synced copy that still has the marker is never used for your own command of that name.
- Removing the marker from a synced autocomplete file has no effect while the command file still has its marker. The shipped command and its embedded script win.
- To customise a shipped command's completion, take over both files.
- A project's autocomplete scripts load only for a registered project. `lodev create` lists each one, as `autocomplete for lodev <name> (web container): .lodev/commands/web/autocomplete/<name>` (or `(runs on this machine)` for host).
- Warnings about custom command files are not printed while the shell completes.

**Shipped autocomplete scripts**

- `composer`, `artisan` and `magento` complete their commands, options and (composer) packages through the tool's own Symfony `_complete`.
- `wp` completes through `wp cli completions`.
- `n98` has none: its `_complete` leaves out n98's own commands.
