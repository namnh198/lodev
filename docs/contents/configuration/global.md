---
title: "Global config — ~/.lodev/config.yaml"
description: "Global settings in ~/.lodev/config.yaml, project homes, data folders and Docker Compose."
sidebar:
  label: Global config
---

All keys optional; defaults shown. Change them with `lodev config-global` (e.g. `--mysql-version=8.4`,
`--enable-services=rabbitmq`) or edit the file. Run `lodev service start` after editing: `lodev start` leaves
the shared services alone while everything it needs is running, so it does not apply these changes.

```yaml
ports:
  http: 80
  https: 443
  dashboard: 10999  # Traefik dashboard
  mysql: ""         # e.g. 3306 to reach MySQL from TablePlus etc.
  mariadb: ""       # e.g. 3307; must differ from mysql
  mongodb: ""       # e.g. 27017 to reach MongoDB from MongoDB Compass etc.
services:           # traefik is always on
  mysql: true
  redis: true
  opensearch: true
  mailpit: true
  phpmyadmin: true
  mariadb: false     # opt-in: `lodev service enable mariadb`, or `services: [mariadb]` in a project
  rabbitmq: false    # opt-in: `lodev service enable rabbitmq`, or `services: [rabbitmq]` in a project
  mongodb: false     # opt-in: `lodev service enable mongodb`, or `services: [mongodb]` in a project
  mongo-express: false # opt-in web UI at https://mongo.test; also starts mongodb
memory:
  opensearch: 2g    # container limit, 1g..4g; JVM heap is half
versions:           # 8 | 8.0 | 8.0.36; only these keys
  mysql: "8.0"
  mariadb: "11.4"
  redis: "7"        # redis:<v>-alpine
  opensearch: "2"     # built as lodev/opensearch:<v> with analysis-icu and analysis-phonetic (Smile ElasticSuite)
  rabbitmq: "4"     # rabbitmq:<v>-management
  mongodb: "8"      # mongo:<v>
node_version: "24"  # Node.js in the web image, installed with n: lts | latest | current | 24 | 22.11;
                    # a project's nodejs: overrides it. Changing it rebuilds the image on the next start
tld: test                     # domains end in .<tld>; first run writes local on Linux, test elsewhere
container_timeout: 120s       # how long start / service start wait for containers to turn healthy
update_check: true  # after a command, say when a newer lodev release exists (asks GitHub at most every 4h);
                    # turn off with `lodev config-global --update-check=false`
auto_create_db: true  # lodev start/restart create the project's database (CREATE DATABASE IF NOT EXISTS);
                      # false = you manage databases yourself (db-import still creates its target)
use_docker_compose_system: false  # true = the system `docker compose` instead of lodev's pinned one
use_docker_buildx_system: false   # true = the system `docker build` (buildx) instead of lodev's pinned one
performance_mode: ""              # bind | none; "" = bind on macOS, none elsewhere (see Performance mode)
data_dir: ~/.lodev/data           # project homes and mounted service data ("" = ~/.lodev/data)
mount_paths:                      # optional, per service: mysql mariadb redis opensearch rabbitmq mongodb
  mysql: /Volumes/SSD/mysql       # absolute, ~/..., or relative to data_dir (redis: redis = <data_dir>/redis)
```

## Project homes and data folders

The container's home (`/home/<user>`) is rebuilt every time the container is created: from the
image's `/etc/skel` (a bashrc with a 10000-line history, `.gitconfig`, a `.bash_aliases` with `ll`), a generated
`~/.my.cnf`, then the files of `~/.lodev/homeadditional` and last those of the project's
`.lodev/homeadditional` (later ones win; nothing is deleted from the home, and the copies are
repeated on every container start; a file the user cannot read is skipped with a warning). Both
folders are mounted read-only. Put composer's auth at
`~/.lodev/homeadditional/.config/composer/auth.json` and your own `.gitconfig` in
`~/.lodev/homeadditional`, which lodev creates; the project's folder is committed with the project,
so keep secrets out of it. `lodev create` scaffolds `.lodev/homeadditional/.bash_aliases.example`
(rename it to `.bash_aliases`; `*.example` files there are not copied). It is in the [trust list](../features/repo-commands.md), and the
container never reads it from the repo: each `lodev start` copies it to
`<data_dir>/web/<name>-web/homeadditional` and mounts that copy (only when the folder exists), so
a change or a new folder takes effect on the next `lodev restart`, and a container Docker restarts
on its own still gets the copy from the last start. An apache project's config is copied the same
way, to `<data_dir>/web/<name>-web/apache`. `lodev remove` deletes both copies.

Only caches and histories outlive the container: `<data_dir>/web/<name>-web` is mounted at
`/var/lib/lodev/home` and keeps exactly `.bash_history`, `.mysql_history`, `.cache/composer`,
`.npm`, `.cache/yarn`, `.local/share/pnpm/store` and `.config/psysh` (through `HISTFILE`,
`MYSQL_HISTFILE`, `COMPOSER_CACHE_DIR`, `npm_config_cache`, `YARN_CACHE_FOLDER` and
`pnpm_config_store_dir`, which `lodev ssh` and `lodev exec` also get, and a `~/.config/psysh`
link); the rest of `~/.cache` and `~/.local` is not kept. lodev creates the folder on
`lodev start` (renaming an older lodev's `<data_dir>/home-<name>` to it, and deleting the old
`homeadditional-<name>` and `apache-<name>` copies); `lodev remove` keeps it (and says where). Other entries an older lodev left in that folder (`.gitconfig`, `.ssh`, `.config/composer`
for composer's `auth.json`...; everything except the cache paths above and `.config/psysh`) are no
longer in the container's home: the first `lodev start` lists them in a warning, once, and moves nothing; copy the ones you need to
`~/.lodev/homeadditional`. Paths are absolute or start with `~/`; changing `data_dir`
(`lodev config-global --data-dir=<path>`) moves nothing.

A shared service keeps its data in its `lodev-<service>` volume unless `mount_paths` gives it a
folder: `lodev config-global --mysql-mount-path=<path>` (likewise `--redis-mount-path`, ...) then
`lodev service start`. The path is absolute, starts with `~` (your home), or is relative to
`data_dir`; it cannot be inside `<data_dir>/web` or overlap another service's folder. On that
`service start`, an empty folder gets the old volume copied in once (the service is stopped first;
the volume is kept as a backup). If a copy fails, empty the folder before retrying. Removing the
path (`--mysql-mount-path=`) goes back to the volume: the next `service start` stops the service,
replaces its volume with the folder it was running on, and renames that folder to
`<folder>.copied-<date>` (delete it once you have checked the data). Copy back needs the service's
container: after `services down`, move the data yourself. Changing a path to another folder moves
nothing (lodev warns). OpenSearch's folder is made world-writable (it runs as UID 1000 and does
not take ownership of its data folder). An older config's `mount_volume: true` means every data
service at `<data_dir>/<service>`; the next `lodev config-global` writes it out as `mount_paths`.

## Docker Compose and Buildx

lodev runs its own pinned Docker Compose (v5.7.0) and Buildx (v0.38.2), so every machine behaves
the same whatever Docker plugins it has, or lacks. The first command that needs one downloads it
to `~/.lodev/bin/` (sha256-checked, with a progress bar); upgrading lodev replaces it when the pin
changes. If the download fails, lodev warns and uses the system plugin. Set
`use_docker_compose_system: true` / `use_docker_buildx_system: true` (or
`lodev config-global --use-docker-compose-system=true`) to always use the system ones.
`lodev version` shows which one runs.

Changing a MySQL/MariaDB version recreates its container on the existing data volume;
`lodev service start` warns when the image changes. Dump your databases first: a major
downgrade (or skipping majors) may not read the old data.

Data lives in named volumes (`lodev-mysql`, `lodev-redis`, `lodev-opensearch`, …; folders with
`mount_paths`), kept across
`poweroff` and `services down`. MongoDB uses one volume, `lodev-mongodb`; the `lodev-mongodb-config`
volume an older lodev made is no longer used (a standalone mongod keeps nothing in it): remove it with
`docker volume rm lodev-mongodb-config`. Older lodev versions left an anonymous volume behind each time
Redis was recreated; list them with `docker volume ls -f dangling=true` and, once you have
checked that nothing else needs them, remove them with `docker volume prune` (it removes every
unused anonymous volume, not only lodev's).
