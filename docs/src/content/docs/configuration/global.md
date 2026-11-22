---
title: "Global config — ~/.lodev/config.yaml"
description: "Global settings, project homes, data folders and Docker Compose."
sidebar:
  label: Global config
---

Machine-wide settings for every project. All keys are optional; defaults are shown below.

- Change them with `lodev config-global` (e.g. `--mysql-version=8.4`, `--enable-services=rabbitmq`). Its flags are in [commands](../features/commands.md).
- Or edit the file.
- Run `lodev service start` after editing. `lodev start` leaves the shared services alone while everything it needs is running, so it does not apply these changes.

Where lodev keeps its files:

- The state folder is `~/.lodev`: this file, `projects.json`, the CA, the downloaded Docker tools and, by default, the data folder.
- `LODEV_HOME` moves that folder.
- It does not move the containers, ports or `/etc/hosts` entries, which every folder shares.

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

### What the container's home holds

The home (`/home/<user>`) is rebuilt every time the container is created, from:

1. The image's `/etc/skel`: a bashrc with a 10000-line history, `.gitconfig`, and a `.bash_aliases` with `ll`.
2. A generated `~/.my.cnf`.
3. The files of `~/.lodev/homeadditional`.
4. The files of the project's `.lodev/homeadditional`. These come last, so they win.

- Nothing is deleted from the home.
- The copies are repeated on every container start.
- A file the user cannot read is skipped with a warning.
- Both folders are mounted read-only.

### Where to put your files

- Composer's auth: `~/.lodev/homeadditional/.config/composer/auth.json`.
- Your own `.gitconfig`: `~/.lodev/homeadditional`. lodev creates this folder.
- The project's `.lodev/homeadditional` is committed with the project, so keep secrets out of it.
- `lodev create` scaffolds `.lodev/homeadditional/.bash_aliases.example`. Rename it to `.bash_aliases`. `*.example` files there are not copied.
- The folder is in the [trust list](../features/repo-commands.md).

### How the copies reach the container

The container never reads `homeadditional` from the repo. Instead:

- Each `lodev start` copies it to `<data_dir>/web/<name>-web/homeadditional` and mounts that copy. The copy is made only when the folder exists.
- A change or a new folder takes effect on the next `lodev restart`.
- A container that Docker restarts on its own still gets the copy from the last start.
- An apache project's config is copied the same way, to `<data_dir>/web/<name>-web/apache`.
- `lodev remove` deletes both copies.

### What outlives the container

- `<data_dir>/web/<name>-web` is mounted at `/var/lib/lodev/home`.
- It keeps exactly these paths:
  - `.bash_history`
  - `.mysql_history`
  - `.cache/composer`
  - `.npm`
  - `.cache/yarn`
  - `.local/share/pnpm/store`
  - `.config/psysh`
- They are set through `HISTFILE`, `MYSQL_HISTFILE`, `COMPOSER_CACHE_DIR`, `npm_config_cache`, `YARN_CACHE_FOLDER` and `pnpm_config_store_dir`. `lodev ssh` and `lodev exec` get the same variables.
- A `~/.config/psysh` link is made too.
- The rest of `~/.cache` and `~/.local` is not kept.

### Upgrading from an older lodev

- `lodev start` creates the folder. It renames an older `<data_dir>/home-<name>` to it, and deletes the old `homeadditional-<name>` and `apache-<name>` copies.
- `lodev remove` keeps the folder, and says where it is.
- Other entries an older lodev left there (`.gitconfig`, `.ssh`, `.config/composer` for composer's `auth.json`, ...) are no longer in the home.
  - The first `lodev start` lists them in one warning. It moves nothing.
  - Copy the ones you need to `~/.lodev/homeadditional`.

### Paths

- Paths are absolute or start with `~/`.
- Changing `data_dir` (`lodev config-global --data-dir=<path>`) moves nothing.

### Shared service data folders

A shared service keeps its data in its `lodev-<service>` volume, unless `mount_paths` gives it a folder.

- Set one: `lodev config-global --mysql-mount-path=<path>` (likewise `--redis-mount-path`, ...), then `lodev service start`.
- The path is absolute, starts with `~` (your home), or is relative to `data_dir`.
- It cannot be inside `<data_dir>/web`, or overlap another service's folder.
- On that `service start`, an empty folder gets the old volume copied in once.
  - The service is stopped first.
  - The volume is kept as a backup.
  - If a copy fails, empty the folder before retrying.
- Remove the path (`--mysql-mount-path=`) to go back to the volume.
  - The next `service start` stops the service and replaces its volume with the folder it was running on.
  - It renames that folder to `<folder>.copied-<date>`. Delete it once you have checked the data.
- Copying back needs the service's container. After `services down`, move the data yourself.
- Changing a path to another folder moves nothing, and lodev warns.
- OpenSearch's folder is made world-writable. It runs as UID 1000 and does not take ownership of its data folder.
- An older config's `mount_volume: true` means every data service at `<data_dir>/<service>`. The next `lodev config-global` writes it out as `mount_paths`.

## Docker Compose and Buildx

- lodev runs its own pinned Docker Compose (v5.7.0) and Buildx (v0.38.2). Every machine behaves the same, whatever Docker plugins it has.
- The first command that needs one downloads it to `~/.lodev/bin/`, sha256-checked, with a progress bar.
- Upgrading lodev replaces it when the pin changes.
- If the download fails, lodev warns and uses the system plugin.
- To always use the system plugins, set `use_docker_compose_system: true` and `use_docker_buildx_system: true`. Or run `lodev config-global --use-docker-compose-system=true`.
- `lodev version` shows which one runs.

### Changing a database version

- Changing a MySQL/MariaDB version recreates its container on the existing data volume.
- `lodev service start` warns when the image changes.
- Dump your databases first. A major downgrade (or skipping majors) may not read the old data.

### Where the data lives

- Named volumes: `lodev-mysql`, `lodev-redis`, `lodev-opensearch`, ... (or folders, with `mount_paths`).
- They are kept across `poweroff` and `services down`.
- MongoDB uses one volume, `lodev-mongodb`.
- The `lodev-mongodb-config` volume an older lodev made is no longer used. A standalone mongod keeps nothing in it. Remove it with `docker volume rm lodev-mongodb-config`.
- Older lodev versions left an anonymous volume behind each time Redis was recreated.
  - List them with `docker volume ls -f dangling=true`.
  - Once you have checked that nothing else needs them, remove them with `docker volume prune`.
  - `docker volume prune` removes every unused anonymous volume, not only lodev's.
