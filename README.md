# lodev

A lightweight, DDEV-like CLI for local PHP development on macOS with Docker.

Unlike DDEV, the heavy services — MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin — run **once,
shared by all projects**. Each project only runs one small PHP container (php-fpm + nginx or
apache), so ten projects cost about the RAM of ten php-fpm processes, not ten databases.

## Install

Requirements: Docker Desktop. [mkcert](https://github.com/FiloSottile/mkcert) is optional
(`brew install mkcert`); without it lodev runs mkcert in Docker and tells you how to trust its CA.

```bash
go build -o lodev . && sudo mv lodev /usr/local/bin/
```

## Quick start

```bash
lodev services up          # Traefik, MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin
cd ~/Sites/myshop
lodev init                 # detects Laravel / WordPress / Magento / plain PHP
lodev start                # → https://myshop.test
lodev magento setup:upgrade
```

`start` asks for your password once to add `.test` names to `/etc/hosts`.

Shared tools: Traefik dashboard <http://localhost:8080>, <https://phpmyadmin.test>,
<https://mailpit.test>. All ports listen on `127.0.0.1` only.

Inside a project container: `DB_HOST=mysql`, `DB_PORT=3306`, `DB_DATABASE=<name>`,
`DB_USERNAME=root`, `DB_PASSWORD=root`, `REDIS_HOST=redis`, `OPENSEARCH_HOST=opensearch`,
`MAIL_HOST=mailpit`, `MAIL_PORT=1025`. PHP `mail()` goes to Mailpit.

## PHP image

All projects share one image, `lodev/php`, with PHP 8.2–8.5; it is built on the first
`lodev start` (about a minute). PHP 7.4, 8.0 and 8.1 add a small image
(`lodev/php:legacy-<ver>`) on top of it the first time a project uses them.

- Change a project's PHP version: edit `php:` in `.lodev/config.yaml`, then `lodev restart`.
- After upgrading lodev, `lodev start` rebuilds the images by itself when their files
  changed. `lodev start --rebuild` forces a clean rebuild (fresh PHP packages).
- Coming from an older lodev that built one image per version? Free the space with
  `docker rmi lodev/php:7.4 lodev/php:8.0 lodev/php:8.1 lodev/php:8.2 lodev/php:8.3 lodev/php:8.4 lodev/php:8.5`
  ("No such image" errors are fine).

## Global config — `~/.lodev/config.yaml`

All keys optional; defaults shown. Run `lodev services up` after editing.

```yaml
ports:
  http: 80
  https: 443
  dashboard: 8080   # Traefik dashboard
  mysql: ""         # e.g. 3306 to reach MySQL from TablePlus etc.
services:           # traefik is always on
  mysql: true
  redis: true
  opensearch: true
  mailpit: true
  phpmyadmin: true
memory:
  opensearch: 2g    # container limit, 1g..4g; JVM heap is half
```

## Project config — `.lodev/config.yaml`

```yaml
name: myshop                 # a-z 0-9 -; database name uses _ instead of -
type: magento                # laravel | wordpress | magento | php
php: "8.3"                   # 7.4 8.0 8.1 8.2 8.3 8.4 8.5
webserver: nginx             # nginx | apache | none (CLI-only, no domain)
docroot: pub
domains:                     # default <name>.test; all must end in .test
  - myshop.test
  - store-en.test
services: [mysql, redis, opensearch]
```

## Commands

| Command | What it does |
|---|---|
| `lodev services up\|down\|status\|logs [svc]` | Manage the shared stack. `down --volumes` deletes all data (asks first) |
| `lodev services enable\|disable <svc>` | Turn a shared service on/off (data is kept) |
| `lodev init [--name --type --php --webserver]` | Create `.lodev/config.yaml` |
| `lodev start [--rebuild]` | Start the project; `--rebuild` rebuilds the PHP image(s) |
| `lodev stop` / `lodev restart` | Stop / restart the project container |
| `lodev delete [--db] [--yes]` | Remove container, hosts entries, cert; `--db` drops the database |
| `lodev list` | All projects with status and domains |
| `lodev ssh` / `lodev exec <cmd>` | Shell / command in the web container |
| `lodev composer\|npm\|node\|artisan\|wp\|magento ...` | Shortcuts |
| `lodev xdebug on\|off\|status` | Toggle Xdebug (port 9003) |

**Known limit:** a running project learns about domains of projects started after it on its
next `lodev start` / `lodev restart`.

## Smoke test

1. `lodev services up`; dashboard, `https://phpmyadmin.test`, `https://mailpit.test` load.
2. Folder `demo` with `index.php` = `<?php phpinfo();`: `lodev init && lodev start`;
   `curl -sI https://demo.test` → 200.
3. Second project `demo2` (`--php 7.4 --webserver apache`); `lodev restart` in demo; from
   demo `lodev exec curl -s -o /dev/null -w '%{http_code}' https://demo2.test` → 200, and back.
4. `lodev init --webserver none` project → `lodev list` shows `CLI`;
   `lodev exec php -r 'echo gethostbyname("mysql");'` prints a `172.30.` address.
5. `lodev xdebug on` → `lodev exec php -m | grep -i xdebug`.
6. `lodev exec php -r 'var_dump(mail("a@b.test","hi","body"));'` → `bool(true)`, mail in Mailpit.
7. A `--php 8.5` project starts; switching a project 8.3 → 8.4 (`php:` + `lodev restart`) builds nothing, → 7.4 builds only `lodev/php:legacy-7.4`.
8. `lodev services disable opensearch` removes the container, keeps the volume; `enable` restores.
9. Move a project folder and `lodev start` there → works.
10. `lodev delete --db --yes` in each test project.
