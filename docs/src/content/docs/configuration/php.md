---
title: "PHP"
description: "The PHP image and the PHP and webserver config files."
---

Which PHP runs in a project, what is in the image, and where to put PHP and webserver settings.

## PHP image

All projects share one image: `lodev/webserver:<lodev version>` (e.g. `lodev/webserver:v0.1.0`).

What it contains:

- PHP 8.2 to 8.6.
- Composer.
- Node.js (`node_version`, default 24) with pnpm, Yarn 1.x and grunt-cli.
- wp-cli, n98-magerun2, nano, vim-tiny, sqlite3 and patch.
- Extensions, for each PHP version: bcmath, curl, gd, imagick, intl, mbstring, mysql, soap, xml and zip. Also redis and xdebug (not on 8.6, which has neither yet), and opcache (built into PHP from 8.5).
- For native extensions and the services: gcc, g++, make, autoconf, pkg-config, `phpize` and `pecl` (`php<ver>-dev` is installed).
- The `mysql` and `redis-cli` clients.
- Mailpit: PHP `mail()` goes to it.

Where the image comes from:

- A stable lodev pulls it from `ghcr.io/namnh198/lodev-webserver:<version>` on the first `lodev start`.
- It builds locally instead (about a minute) when:
  - the build is nightly or dev
  - `node_version` is not 24
  - you pass `--rebuild-base`
  - the pull fails
- PHP 7.4, 8.0 and 8.1 add a small image on top of it, the first time a project uses them: `lodev/webserver:<lodev version>-legacy-<ver>`.

### Security fixes

- Every PHP version is built with the PHP2Shell fixes (CVE-2026-109618/109619).
  - 8.2.35, 8.3.36, 8.4.27, 8.5.12, 8.6.0 or newer.
  - deb.sury.org's backports for 7.4, 8.0 and 8.1.
- The build stops if a version comes out older.

### Xdebug on 8.6

- PHP 8.6 has no Xdebug and no phpredis yet. deb.sury.org does not package them.
- On 8.6, `lodev xdebug on` says so and changes nothing.
- On 8.6, `lodev start` ignores `enable_xdebug` with a warning.

## Changing and rebuilding

- Change a project's PHP version: `lodev config --php-version=8.4`, or edit `php:` in `.lodev/config.yaml`. Then run `lodev restart`.
- After upgrading lodev, the first `lodev start` builds the images for the new version.
- `lodev start --rebuild` rebuilds the project image without the build cache.
- `lodev start --rebuild-base` also rebuilds the shared PHP image(s) from scratch. It fetches fresh PHP packages, so it is slow.

### Old images

- Images of older lodev versions are kept.
- List them with `docker images lodev/webserver`.
- Free the space with `docker rmi lodev/webserver:<old version>`.
- lodev before `lodev/webserver` used `lodev/php`. Remove those with `docker rmi $(docker images -q lodev/php)`.

## PHP and webserver config

| Path | Effect |
|---|---|
| `.lodev/php/*.ini` | Extra php.ini settings, over lodev's defaults: errors shown (`display_errors=On`, `E_ALL`), `memory_limit=-1`, `max_execution_time=360`, `post_max_size`/`upload_max_filesize=1024M`, `max_input_vars=100000`, no output buffering, OPcache sized for Magento (512M, 130000 files) and a 10M realpath cache; php-fpm runs `ondemand` with up to 20 workers |
| `.lodev/php/prepend.php` | PHP `auto_prepend_file` for web and CLI (e.g. set `$_SERVER['MAGE_RUN_CODE']`); created by `lodev start` |
| `.lodev/nginx/*.conf` or `.lodev/apache/*.conf` | Snippets included in the server / vhost block |
| `.lodev/nginx-webserver.conf`, `.lodev/apache-webserver.conf` | The whole webserver config (generated, one file per webserver) |

### Applying changes

- Apply with `lodev restart`. None of these builds an image.
- `lodev create` scaffolds `*.example` files in each folder.
- Switching `webserver:` does not touch the other webserver's file.

### Taking over a generated config

- A generated webserver conf starts with `# lodev-generated: delete this line to manage this file yourself`.
- Delete that line to take the file over.
- lodev then stops regenerating it, and it is no longer git-ignored.

### Client IP

- PHP's `REMOTE_ADDR` is the client from `X-Forwarded-For`, past Traefik and Varnish.
- From this machine, that is `172.30.0.1` (the lodev network gateway).
- To test GeoIP, send a header: `curl -k -H 'X-Forwarded-For: 8.8.8.8' https://<project>.test/`.

### Git and old files

- lodev no longer writes your project's root `.gitignore`.
- Old `.lodev/compose.yaml` and `.lodev/webserver.conf` lines there can be removed.
- lodev keeps `.lodev/.gitignore` itself.

### Upgrading from an older lodev

Run `lodev start` once in each project, before `stop`, `exec` or `ssh`. They use the generated `lodev.compose-full.yml`.
