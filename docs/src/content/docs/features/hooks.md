---
title: "Hooks"
description: "Run commands at lifecycle points of a project."
---

Runs steps in the web container at four moments in a project's life.

- Runs as your user, in `working_dir`.
- Needs the web container running (see the last section).

| Event | When |
|---|---|
| `post-start` | after `lodev start` / `restart`, once the web container is healthy |
| `pre-stop` | before `lodev stop` / `restart` stops the project (a failure keeps it running) |
| `post-db-import` | after `lodev db-import` into the project's own database |
| `post-snapshot-restore` | after `lodev snapshot restore` |

## Defining steps

Steps come from two places, in this order:

1. Config steps in `.lodev/config.yaml`.
2. Every file in `.lodev/hooks/<event>.d/`, in name order.

```yaml
hooks:
  post-start:
    - composer install
    - script: .lodev/hooks/warm-cache.sh
```

- A step is a shell command, or a script in the project (`script:`).
- A file in `.lodev/hooks/<event>.d/` runs after the config's steps, e.g. `.lodev/hooks/post-start.d/10-composer.sh`.
- Skipped in `.d/`: dot files and `.example` files.
- A script must be executable (`chmod +x`). Its shebang picks the interpreter.
- A command step runs with `sh -c`.

## Running

- Steps get `LODEV_HOOK` and `LODEV_PROJECT`.
- A failing step stops the remaining ones, and lodev exits with an error.
- The command itself is not undone.
- A hook needs the web container running. Without it, the step fails with `hook <event>: lodev-<name>-web is not running`.

## Skipping hooks

`--no-hooks` skips them:

- `start`: `post-start`
- `stop`: `pre-stop`
- `restart`: both
- `db-import`: `post-db-import`
- `snapshot restore`: `post-snapshot-restore`
