---
title: "Hooks"
description: "Run commands at lifecycle points of a project."
---

Steps lodev runs in the web container (as your user, in `working_dir`) at four moments:

| Event | When |
|---|---|
| `post-start` | after `lodev start` / `restart`, once the web container is healthy |
| `pre-stop` | before `lodev stop` / `restart` stops the project (a failure keeps it running) |
| `post-db-import` | after `lodev db-import` into the project's own database |
| `post-snapshot-restore` | after `lodev snapshot restore` |

A step is a shell command or a script in the project, in `.lodev/config.yaml`:

```yaml
hooks:
  post-start:
    - composer install
    - script: .lodev/hooks/warm-cache.sh
```

and every file in `.lodev/hooks/<event>.d/` (e.g. `.lodev/hooks/post-start.d/10-composer.sh`)
runs too, after the config's steps, in name order (`.example` and dot files are skipped). A script
must be executable (`chmod +x`); its shebang picks the interpreter. Steps get `LODEV_HOOK` and
`LODEV_PROJECT`. A failing step stops the remaining ones and lodev exits with an error; the command
itself is not undone. `--no-hooks` on `start`, `restart`, `stop` and `db-import` skips them.
