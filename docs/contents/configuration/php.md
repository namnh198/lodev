---
title: "PHP"
description: "The PHP image and the PHP and webserver config files."
---

## PHP image

All projects share one image, `lodev/webserver:<lodev version>` (e.g. `lodev/webserver:v0.1.0`),
with PHP 8.2–8.6, Composer, Node.js (`node_version`, default 24) with pnpm, Yarn 1.x and
grunt-cli, wp-cli, n98-magerun2, nano, vim-tiny, sqlite3 and patch. For native extensions and the
services: gcc, g++, make, autoconf, pkg-config, `phpize` and `pecl` (`php<ver>-dev` is installed),
and the `mysql` and `redis-cli` clients. A stable lodev pulls it from `ghcr.io/namnh198/lodev-webserver:<version>` on the
first `lodev start`; nightly and dev builds, a `node_version` other than 24, `--rebuild-base` or a
failed pull build it locally instead (about a minute). PHP 7.4, 8.0 and
8.1 add a small image (`lodev/webserver:<lodev version>-legacy-<ver>`) on top of it the first
time a project uses them.

Every PHP version is built with the PHP2Shell fixes (CVE-2026-109618/109619): 8.2.35, 8.3.36,
8.4.27, 8.5.12, 8.6.0 or newer, and deb.sury.org's backports for 7.4, 8.0 and 8.1. The build
stops if a version comes out older. PHP 8.6 has no Xdebug and no phpredis yet (deb.sury.org does
not package them): on 8.6, `lodev xdebug on` says so and changes nothing, and `lodev start`
ignores `enable_xdebug` with a warning.

- Change a project's PHP version: `lodev config --php-version=8.4` (or edit `php:` in `.lodev/config.yaml`), then `lodev restart`.
- After upgrading lodev, the first `lodev start` builds the images for the new version.
  `lodev start --rebuild` rebuilds the project image without the build cache; `--rebuild-base` also
  rebuilds the shared PHP image(s) from scratch (fresh PHP packages; slow).
- Images of older lodev versions are kept. List them with `docker images lodev/webserver`
  and free the space with `docker rmi lodev/webserver:<old version>`; lodev before
  `lodev/webserver` used `lodev/php` (`docker rmi $(docker images -q lodev/php)`).

## PHP and webserver config

| Path | Effect |
|---|---|
| `.lodev/php/*.ini` | Extra php.ini settings, over lodev's defaults: errors shown (`display_errors=On`, `E_ALL`), `memory_limit=-1`, `max_execution_time=360`, `post_max_size`/`upload_max_filesize=1024M`, `max_input_vars=100000`, no output buffering, OPcache sized for Magento (512M, 130000 files) and a 10M realpath cache; php-fpm runs `ondemand` with up to 20 workers |
| `.lodev/php/prepend.php` | PHP `auto_prepend_file` for web and CLI (e.g. set `$_SERVER['MAGE_RUN_CODE']`); created by `lodev start` |
| `.lodev/nginx/*.conf` or `.lodev/apache/*.conf` | Snippets included in the server / vhost block |
| `.lodev/nginx-webserver.conf`, `.lodev/apache-webserver.conf` | The whole webserver config (generated, one file per webserver) |

Apply with `lodev restart`; none of these builds an image. A generated webserver conf starts with
`# lodev-generated: delete this line to manage this file yourself`; delete that line to take the
file over (lodev stops regenerating it and it is no longer ignored). Switching `webserver:` does
not touch the other webserver's file. `lodev create` scaffolds `*.example` files in each folder.

PHP's `REMOTE_ADDR` is the client from `X-Forwarded-For`, past Traefik and Varnish. From this machine
that is `172.30.0.1` (the lodev network gateway); to test GeoIP, send a header:
`curl -k -H 'X-Forwarded-For: 8.8.8.8' https://<project>.test/`.

lodev no longer writes your project's root `.gitignore`; old `.lodev/compose.yaml` /
`.lodev/webserver.conf` lines there can be removed (lodev keeps `.lodev/.gitignore` itself).

**Upgrading from an older lodev:** run `lodev start` once in each project before `stop`, `exec`
or `ssh` (they use the generated `lodev.compose-full.yml`).
