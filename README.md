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
nodejs: "20"                 # optional: lts | latest | current | 20 | 20.11 | 20.11.1 (installed with n)
php_extensions: [imagick]    # optional: a-z 0-9 _ ; installed for the project's PHP version
packages: [ffmpeg]           # optional: extra apt packages
```

`nodejs`, `php_extensions`, `packages` and non-example `Dockerfile*` files in `.lodev/web-images/`
make lodev build a project image `lodev/<name>-web` on top of the PHP image. Projects without
any of them use the shared PHP image and build nothing. The image is rebuilt on the next
`lodev start` / `lodev restart` after one of them changes; `lodev start --rebuild` forces it.

## Customizing the web image

Put `Dockerfile*` files in `.lodev/web-images/` (added in name order after the generated
steps; `*.example` files are ignored). Plain Dockerfile instructions, no `FROM`:

```dockerfile
# .lodev/web-images/Dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends poppler-utils && rm -rf /var/lib/apt/lists/*
```

Editing a file rebuilds the project image on the next `lodev start` / `restart`; leaving it
alone builds nothing. `lodev init` scaffolds `Dockerfile.example`.

## Extra services

Any `.lodev/*.compose.yml` (or `.yaml`) is merged into the project's compose stack
(`.lodev/lodev.compose-full.yml`, generated and ignored by version control):

```yaml
# .lodev/redis-commander.compose.yml
services:
  redis-commander:
    image: rediscommander/redis-commander
    environment: [REDIS_HOSTS=local:redis:6379]
    networks: [lodev, default]   # lodev: reach redis/mysql; default: talk to web
```

Don't name an extra service `mysql`, `redis`, `opensearch`, `mailpit` or `phpmyadmin`: it would
collide with the shared ones on the `lodev` network. On the `lodev` network the web container is
reachable as `lodev-<name>-web` (the bare `web` alias exists on every project's `lodev` network, so
it is ambiguous); `web` works only on the project's `default` network. `lodev init` scaffolds `example.compose.yml.example`.

## PHP and webserver config

| Path | Effect |
|---|---|
| `.lodev/php/*.ini` | Extra php.ini settings (`memory_limit=4G`) |
| `.lodev/nginx/*.conf` or `.lodev/apache/*.conf` | Snippets included in the server / vhost block |
| `.lodev/nginx-webserver.conf`, `.lodev/apache-webserver.conf` | The whole webserver config (generated, one file per webserver) |

Apply with `lodev restart`; none of these builds an image. A generated webserver conf starts with
`# lodev-generated: delete this line to manage this file yourself`; delete that line to take the
file over (lodev stops regenerating it and it is no longer ignored). Switching `webserver:` does
not touch the other webserver's file. `lodev init` scaffolds `*.example` files in each folder.

lodev no longer writes your project's root `.gitignore`; old `.lodev/compose.yaml` /
`.lodev/webserver.conf` lines there can be removed (lodev keeps `.lodev/.gitignore` itself).

**Upgrading from an older lodev:** run `lodev start` once in each project before `stop`, `exec`
or `ssh` (they use the generated `lodev.compose-full.yml`).

## Commands

| Command | What it does |
|---|---|
| `lodev services up\|down\|status\|logs [svc]` | Manage the shared stack. `down --volumes` deletes all data (asks first) |
| `lodev services enable\|disable <svc>` | Turn a shared service on/off (data is kept) |
| `lodev init [--name --type --php --webserver --docroot --no-ask]` | Create `.lodev/config.yaml`. On a terminal an interactive form asks name/type/PHP/docroot/webserver (detected values preselected); `--no-ask` skips it; `--docroot .` = project root |
| `lodev start [--rebuild]` | Start the project; `--rebuild` rebuilds the PHP image(s) and the project image |
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
10. `nodejs: "20"` in `.lodev/config.yaml` + `lodev restart` → `lodev node -v` prints `v20.x`; a second `lodev restart` builds nothing.
11. `php_extensions: [imagick]` + `lodev restart` → `lodev exec php -m` lists `imagick`; after switching `php:` it is installed for that version.
12. `RUN touch /lodev-ok` in `.lodev/web-images/Dockerfile` + `lodev restart` → `lodev exec ls /lodev-ok` works; a further restart builds nothing.
13. The `redis-commander` file above as `.lodev/redis-commander.compose.yml` + `lodev start` → the service is up and `lodev exec curl -s redis-commander:8081` answers.
14. `.lodev/php/my.ini` with `memory_limit=4G` + `lodev restart` → `lodev exec php -i | grep memory_limit` shows `4G`, no image build.
15. `lodev init` in a folder containing an `artisan` file (real terminal) asks name/type/PHP/docroot/webserver with `laravel` and `public` preselected.
16. `lodev delete --db --yes` in each test project.
