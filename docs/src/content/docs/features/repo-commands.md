---
title: "Commands from the project's repo"
description: "Commands a repo asks lodev to run, and the trust list that guards them."
---

A cloned repo can make lodev run commands: hooks (config steps, the scripts they name and the
`.lodev/hooks/<event>.d/` files), cron lines, `web_extra_daemons`, a nextjs project's `dev`, `build` and `start`/`preview` scripts (or its `.lodev/daemons/nextjs.sh` once you take it over), extra `.lodev/*.compose.yml`
files, `.lodev/web-images/` parts, `.lodev/homeadditional/` files (a symlinked folder is
followed; not `.example` ones), the project's `environment:` (from `config.yaml` and its overrides),
the names of the variables in `.lodev/.env` (a repo can commit it; values are not shown, it holds secrets),
your own apache config: the files in `.lodev/apache/` (not `.example` ones) and
`.lodev/apache-webserver.conf` once it no longer carries lodev's marker, and the
[custom commands](custom-commands.md) in `.lodev/commands/web/` and `.lodev/commands/host/`, with their
[autocomplete scripts](custom-commands.md#autocomplete-scripts) (which run when you press TAB). lodev lists them and asks
`Run these commands? [Y/n]` (enter approves) once, on `lodev create` (a folder with a `.lodev/config.yaml`, or one
whose repo already has `.lodev` files; nothing to run asks nothing). A declined question registers
nothing. Where lodev cannot ask (no terminal) it fails with the list; pass `--yes` to `create` to
approve it. Control characters in the list (escape sequences, carriage returns, bidi
overrides) are shown as escapes like `\x1b`, so the repo's text cannot hide a line. Nothing is stored: `start`, `restart` and the hooks run by `stop`, `db-import` and
`snapshot restore` do not ask again.

Accepted risk: a `git pull` that adds or changes a hook, daemon, compose file, web-image part,
homeadditional or apache file runs at the next start without a question. Review what a pull
brings into `.lodev/` before starting the project.
A custom command runs when you type `lodev <name>`, and a project file replaces a shipped or
global command of the same name. A `git pull` that adds `.lodev/commands/host/composer` makes your
next `lodev composer` run that script on your machine, outside the container, as you, with no
question; the same goes for `npm`, `php` and the other shipped names. Projects registered before
you upgraded to custom commands load the `.lodev/commands/` their repo already holds at once,
without it having been listed. Review `.lodev/commands/` after a pull or an upgrade. Your own
`~/.lodev/commands/` files are not listed.
