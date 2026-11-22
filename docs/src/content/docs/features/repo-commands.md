---
title: "Repo commands"
description: "Commands a repo asks lodev to run, and the trust list that guards them."
---

A cloned repo can make lodev run commands. This page lists them and how lodev asks before it runs them.

## What a repo can make lodev run

- Hooks:
  - config steps, and the scripts they name
  - the `.lodev/hooks/<event>.d/` files
- Cron lines.
- `web_extra_daemons`.
- A nextjs project's `dev`, `build` and `start`/`preview` scripts, or its `.lodev/daemons/nextjs.sh` once you take it over.
- Extra `.lodev/*.compose.yml` files.
- `.lodev/web-images/` parts.
- `.lodev/homeadditional/` files:
  - a symlinked folder is followed
  - `.example` files are not listed
- The project's `environment:`, from `config.yaml` and its overrides.
- The names of the variables in `.lodev/.env`:
  - a repo can commit it
  - values are not shown, because it holds secrets
- Your own apache config:
  - the files in `.lodev/apache/` (not `.example` ones)
  - `.lodev/apache-webserver.conf`, once it no longer carries lodev's marker
- The [custom commands](custom-commands.md) in `.lodev/commands/web/` and `.lodev/commands/host/`, with their [autocomplete scripts](custom-commands.md#autocomplete-scripts). Those run when you press TAB.

## How lodev asks

- lodev lists them, then asks `Run these commands? [Y/n]`. Enter approves.
- It asks once, on `lodev create`. That covers a folder with a `.lodev/config.yaml`, or one whose repo already has `.lodev` files.
- Nothing to run means no question.
- A declined question registers nothing.
- With no terminal, it fails and prints the list. Pass `--yes` to `create` to approve it.
- Control characters in the list are shown as escapes, like `\x1b`. Escape sequences, carriage returns and bidi overrides cannot hide a line.
- Nothing is stored. `start`, `restart`, and the hooks run by `stop`, `db-import` and `snapshot restore` do not ask again.

## Accepted risk

- A `git pull` that adds or changes a hook, daemon, compose file, web-image part, homeadditional or apache file runs at the next start, without a question.
- Review what a pull brings into `.lodev/` before you start the project.

## Custom commands

- A custom command runs when you type `lodev <name>`.
- A project file replaces a shipped or global command of the same name.
- A `git pull` that adds `.lodev/commands/host/composer` makes your next `lodev composer` run that script:
  - on your machine, outside the container
  - as you
  - with no question
- The same goes for `npm`, `php` and the other shipped names.
- Projects registered before you upgraded to custom commands load the `.lodev/commands/` their repo already holds, at once. It was never listed for you.
- Review `.lodev/commands/` after a pull or an upgrade.
- Your own `~/.lodev/commands/` files are not listed.
