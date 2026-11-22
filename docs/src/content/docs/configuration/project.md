---
title: "Project config — .lodev/config.yaml"
description: "Every key of a project's .lodev/config.yaml, with its type, default and allowed values, and the local overrides files."
sidebar:
  label: Project config
---

Reference for every key in a project's `.lodev/config.yaml`, and the local override files.

- Most keys: change them with `lodev config`. Its flags are in [commands](../features/commands.md).
  - Examples: `--php-version=8.4`, `--add-services=redis`, `--nodejs-version=""`.
- `lodev config` keeps your comments.
- It saves nothing when the result is invalid.
- `lodev config` alone prints the current values.
- Keys without a flag are edited in the file: `name`, `working_dir`, `php_ide_config`, `cron_jobs`, `hooks`, `environment`, `web_extra_daemons`, `web_extra_exposed_ports`, `lodev_version_constraint`.
- An invalid value stops the commands that read the file, and they print the reason.

## Keys

- `string` is text, `list` is a YAML list, `bool` is `true` or `false`.
- A key marked "local" is written to `.lodev/config.local.yaml` by its command. See Local overrides below.

| Key | Type | Default | Allowed values and rules |
|---|---|---|---|
| `name` | string | `create` uses the folder name | Required. a-z, 0-9, `-`; `lodev-services` is reserved. The database is named after it with `_` for `-`. |
| `type` | string | `create` detects it | Required. `laravel`, `wordpress`, `magento`, `nextjs` or `php`. |
| `php` | string | `create` writes `"8.4"` | Required. `7.4`, `8.0` to `8.6`. Quote it so YAML keeps it as text. |
| `webserver` | string | `nginx` | `nginx`, `apache` or `none` (CLI only, no domain). `nextjs` needs `nginx`; varnish and exposed ports need a webserver. |
| `docroot` | string | `public` for laravel, `pub` for magento, else the project root | A folder inside the project: A-Z, a-z, 0-9, `.`, `_`, `-`, `/`. Empty means the project root. |
| `container_port` | int | `3000` for `nextjs`; unset otherwise | `nextjs` only: 1 to 65535, not 80 (nginx stays on 80). Other types accept unset or 80. |
| `domains` | list | `[<name>.<tld>]` | Lowercase host names ending in `.<tld>`, the `tld` of the global config. The shared services' names (`mailpit`, `phpmyadmin`, `rabbitmq`, `mongo`) are reserved. |
| `services` | list | none; `create` adds the defaults of the type | Shared services: `mysql`, `mariadb`, `redis`, `opensearch`, `mailpit`, `phpmyadmin`, `rabbitmq`, `mongodb`, `mongo-express`. Not both `mysql` and `mariadb`. `create` adds magento: `mysql`, `redis`, `opensearch`; laravel: `mysql`, `redis`; wordpress and php: `mysql`. |
| `nodejs` | string | unset: the global `node_version` (24) | `lts`, `latest`, `current`, `20`, `20.11` or `20.11.1`. Setting it builds a project image. |
| `php_extensions` | list | none | Extension names: a-z, 0-9, `_`. Installed for the project's `php`. Builds a project image. |
| `packages` | list | none | Extra apt packages: at least 2 characters of a-z, 0-9, `+`, `.`, `-`, starting with a letter or digit. Builds a project image. |
| `user` | string | your host username | The shell user in the web container. Lowercased; other characters become `_`; at most 32 characters; `root` and `www-data` get `_user`. |
| `working_dir` | string | `/app` | Absolute folder where the project is mounted in the web container (a-z, 0-9, `.`, `_`, `-`). Not inside the container's system folders such as `/etc`, `/usr` or `/tmp`. |
| `php_ide_config` | string | the project `name` | The `serverName` PhpStorm maps (one line). |
| `enable_xdebug` | bool | `false` | Local. `lodev xdebug on` and `off` set it. |
| `opcache_validate_timestamps` | bool | unset: PHP's default, `true` | Local. `false` turns off file timestamp checks (for benchmarks); restart after code changes. |
| `varnish` | bool | `false` | Puts a Varnish container in front of the web server. Needs a webserver. |
| `varnish_version` | string | `7.6` | The varnish image tag, e.g. `7.6` or `9.1`. |
| `cron` | bool | `false` | Runs `cron_jobs`, or the type's default job. A `php` project needs `cron_jobs`. |
| `cron_jobs` | list | the type's default job | Crontab lines: five schedule fields and a command, or an `@` macro and a command. |
| `performance_mode` | string | the global value: `bind` on macOS, `none` elsewhere | `bind` or `none`. |
| `volume_dirs` | list | none | More project folders kept in Docker volumes, besides the type's. Relative paths, not `.lodev`, not nested in each other. Ignored when `performance_mode` is `none`. |
| `web_extra_daemons` | list of `{name, command, directory}` | none | `name`: a-z, 0-9, `-`, unique, not starting with `-`; `nextjs` is reserved. `command`: required, one line. `directory`: optional absolute path, default `working_dir`. |
| `web_extra_exposed_ports` | list of `{name, container_port}` | none | Needs a webserver. `name`: a host label (a-z, 0-9, `-`, not starting with `svc-`), served at `<name>.<first domain>`. `container_port`: 1 to 65535, not 80; names and ports unique. |
| `environment` | map of string to string | none | Variables for the web container. Names: A-Z, a-z, 0-9, `_`, not starting with a digit. `LODEV_*` is reserved. `.lodev/.env` overrides a value. |
| `hooks` | map of event to list | none | Events: `post-start`, `pre-stop`, `post-db-import`, `post-snapshot-restore`. A step is a command string or `{script: <path inside the project>}`. See [hooks](../features/hooks.md). |
| `lodev_version_constraint` | string | none | A version range such as `">= 1.4.0, < 2"`. `start` and `restart` refuse another lodev. |

## Annotated example

Every key, with an example value. This file is valid for a magento project. The comments mark the
keys that only some types or settings accept.

```yaml
name: myshop                     # a-z 0-9 -; the database is myshop
type: magento                    # laravel | wordpress | magento | nextjs | php
php: "8.4"                       # 7.4 8.0 8.1 8.2 8.3 8.4 8.5 8.6
webserver: nginx                 # nginx | apache | none (CLI only, no domain)
docroot: pub                     # a folder inside the project; empty = the project root
# container_port: 3000           # nextjs only: the Next.js dev server's port
domains:                         # default <name>.<tld>; each must end in .<tld>
  - myshop.test
  - store-en.test
services: [mysql, redis, opensearch]   # mariadb instead of mysql (not both)
nodejs: "20"                     # optional: lts | latest | current | 20 | 20.11 | 20.11.1
php_extensions: [ldap]           # optional: extensions for the project's PHP
packages: [ffmpeg]               # optional: extra apt packages
user: dev                        # shell user in the web container; default: your host username
working_dir: /app                # where the project is mounted in the web container
php_ide_config: myshop           # PhpStorm's serverName; default: name
# enable_xdebug: true            # local: lodev xdebug on/off writes .lodev/config.local.yaml
# opcache_validate_timestamps: false   # local: benchmarks only
varnish: true                    # Varnish in front of the web server
varnish_version: "7.6"           # the varnish image tag
cron: true                       # run cron jobs in the web container
cron_jobs:                       # empty: the type's default job
  - '*/5 * * * * php bin/magento indexer:reindex >> var/log/reindex.log 2>&1'
performance_mode: bind           # bind | none; default: the global value
volume_dirs: [pub/media]         # more folders in Docker volumes (bind mode)
web_extra_daemons:               # long-running processes, run under supervisor
  - name: vite                   # a-z 0-9 -, unique
    command: npm run dev -- --host 0.0.0.0 --port 5173 --strictPort
    directory: /app              # optional: absolute; default working_dir
web_extra_exposed_ports:         # served at https://vite.<first domain>
  - name: vite
    container_port: 5173         # 1 to 65535, not 80
environment:                     # variables for the web container
  MAGE_RUN_CODE: default
hooks:                           # steps run in the web container
  post-start:
    - composer install           # a shell command
    - script: .lodev/hooks/warm-cache.sh   # a script inside the project
lodev_version_constraint: ">= 1.4.0, < 2"   # start and restart refuse another lodev
```

- A nextjs project uses `type: nextjs` with `webserver: nginx`, and may set `container_port`. See [Next.js](../features/nextjs.md).
- A `php` project with `cron: true` needs `cron_jobs`.
- For the shared services and the global settings, see [global config](global.md).

## Local overrides — `.lodev/config.*.yaml`

Any `.lodev/config.<something>.yaml` (e.g. `config.local.yaml`) is laid over `config.yaml`:

- It is read in name order, every time lodev reads the project.
- Mappings merge key by key. `environment: {B: "3"}` changes only `B`.
- Any other value replaces the one below it. Lists too.
- `name` cannot be overridden.
- The files are git-ignored (`.lodev/.gitignore`), so they hold per-machine settings.

What `lodev config` does with overrides:

- It shows the merged values and the override files in use.
- It checks your change against the merged result.
- It edits `config.yaml` only.
- It warns when an override wins, e.g. `php is overridden by .lodev/config.local.yaml`.

The two local keys are the exception:

- `enable_xdebug` and `opcache_validate_timestamps` are written to `.lodev/config.local.yaml` only, by:
  - `lodev xdebug on|off|toggle`
  - `lodev config --enable-xdebug=true|false`
  - `lodev config --opcache-validate-timestamps=true|false`
- Turning one off removes it from `config.local.yaml`. If `config.yaml` or another override still says otherwise, it sets `false` or `true` there instead.
- An override that sorts after `config.local.yaml` still wins, with the same warning.
- `lodev start` warns when `config.yaml` itself holds one of these keys.
- lodev never edits `config.yaml` to move a local key.
- `lodev config-global --tld` also moves the `domains` in override files.

## Project image

These make lodev build a project image, `lodev/<name>-web`, on top of the PHP image:

- `nodejs`
- `php_extensions`
- `packages`
- non-example `Dockerfile*` files in `.lodev/web-images/`

Projects with none of them use the shared PHP image and build nothing.

- The image is rebuilt on the next `lodev start` or `lodev restart` after one of them changes.
- `lodev start --rebuild` forces a rebuild.
- apt and Node downloads are kept in BuildKit cache mounts. A layer that reruns (e.g. after the PHP image changed) installs without downloading again.
- `--rebuild` uses no cache at all, so it downloads everything.
