---
paths:
  - "assets/webserver/**"
  - "assets/templates/**"
  - "internal/templates/**"
  - "cmd/project.go"
---

# The web container

- The image tag is a hash of everything under `assets/webserver/` (skel dotfiles included), so
  any change there rebuilds the image on the next `lodev start`; check a change with
  `docker run` against the built image before calling it done.
- Host → container contract (change both sides together, plus the golden files):
  - `<data_dir>/web/<name>-web` → `/var/lib/lodev/home` (rw: caches and histories only, through
    `HISTFILE`, `MYSQL_HISTFILE`, `COMPOSER_CACHE_DIR`, `npm_config_cache`, `YARN_CACHE_FOLDER`,
    `pnpm_config_store_dir`).
  - its `homeadditional/` and `~/.lodev/homeadditional` → `/etc/lodev/homeadditional/{project,global}:ro`.
  - its `apache/` → `/etc/lodev/webserver.conf` and `/etc/apache2/custom.d` (`:ro`).
- `entrypoint.sh` order: `useradd -m` from `/etc/skel`, `.my.cnf`, global homeadditional, project
  homeadditional (later wins, nothing deleted), psysh link. It runs on every container start.
- `lodev ssh` is a non-login `bash`: only `.bashrc` is read. Hooks, cron and daemons run `sh -c`,
  so aliases and `.bashrc` never reach them; their settings belong in `environment:`.
- Start logs stay clean: a change that adds a warning to `docker logs` (AH00558, rehash,
  useradd) is a regression.
