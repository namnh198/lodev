---
title: "Project config — .lodev/config.yaml"
description: "Every key of a project's .lodev/config.yaml, and the local overrides files."
sidebar:
  label: Project config
---

Change any key but `name` with `lodev config` (e.g. `--php-version=8.4 --nodejs-version=24`,
`--add-services=redis`, `--nodejs-version=""` to remove a key); it keeps your comments and
saves nothing when the result is invalid. `lodev config` alone prints the current values.

```yaml
name: myshop                 # a-z 0-9 -; database name uses _ instead of -
type: magento                # laravel | wordpress | magento | nextjs | php
php: "8.4"                   # 7.4 8.0 8.1 8.2 8.3 8.4 8.5 8.6
webserver: nginx             # nginx | apache | none (CLI-only, no domain)
docroot: pub                 # a folder inside the project: A-Z a-z 0-9 . _ - /
container_port: 3000         # optional, type nextjs only: the Next dev server's port (default 3000; nginx stays on 80)
domains:                     # default <name>.<tld>; all must end in .<tld> (tld in the global config)
  - myshop.test
  - store-en.test
services: [mysql, redis, opensearch]   # mariadb instead of mysql for MariaDB (not both)
nodejs: "20"                 # optional: lts | latest | current | 20 | 20.11 | 20.11.1 (installed with n)
php_extensions: [ldap]       # optional: a-z 0-9 _ ; installed for the project's PHP version
packages: [ffmpeg]           # optional: extra apt packages
user: kuroko                 # optional: shell user in the web container; default = your host username
working_dir: /app            # optional: where the project is mounted in the web container (default /app)
php_ide_config: myshop       # optional: PHP_IDE_CONFIG serverName for PhpStorm (default: name)
enable_xdebug: true          # optional: Xdebug on at every start (default false); a local key: lodev xdebug on/off keeps it in .lodev/config.local.yaml
opcache_validate_timestamps: false  # optional: false turns opcache.validate_timestamps off (benchmarking); a local key (.lodev/config.local.yaml), and restart after code changes
varnish: true                # optional: Varnish in front of the web server (default false)
varnish_version: "7.6"       # optional: the varnish image tag (default 7.6)
cron: true                   # optional: run cron jobs in the web container (default false)
performance_mode: bind       # optional: bind | none (default: the global value; see Performance mode)
volume_dirs: [pub/media]     # optional: more folders in Docker volumes; ignored when performance_mode is none
web_extra_daemons:           # optional: long-running processes supervisor runs in the web container (see Daemons)
  - name: vite               # a-z 0-9 -, unique
    command: npm run dev -- --host 0.0.0.0 --port 5173 --strictPort
    directory: /app          # optional: absolute; default working_dir
web_extra_exposed_ports:     # optional: <name>.<first domain> routes to a web container port (see Exposed ports)
  - name: vite               # a host name label, unique; here vite.<first domain>
    container_port: 5173     # 1-65535, not 80
cron_jobs:                   # optional: empty = the type's default job (see Cron)
  - '*/5 * * * * php bin/magento indexer:reindex >> var/log/reindex.log 2>&1'
environment:                 # optional: variables for the web container (see Environment variables)
  MAGE_RUN_CODE: default
lodev_version_constraint: ">= 1.4.0, < 2"   # optional: start/restart refuse another lodev
```

## Local overrides — `.lodev/config.*.yaml`

Any `.lodev/config.<something>.yaml` (e.g. `config.local.yaml`) is laid over `config.yaml`, in name
order, every time lodev reads the project. Mappings merge key by key (`environment: {B: "3"}` changes
only `B`); any other value, a list included, replaces the one below it. `name` cannot be overridden.
These files are git-ignored (`.lodev/.gitignore`), so they hold per-machine settings.

`lodev config` shows the merged values and the override files in use, and checks your change
against the merged result. It edits `config.yaml` only and warns when an override wins
(`php is overridden by .lodev/config.local.yaml`), except for the two local keys
`enable_xdebug` and `opcache_validate_timestamps`: `lodev xdebug on|off|toggle` and
`lodev config --enable-xdebug=true|false` / `--opcache-validate-timestamps=true|false` write them
to `.lodev/config.local.yaml` only (turning one off removes it there, or sets it `false`/`true` there
when `config.yaml` or another override still says otherwise; an override sorting after
`config.local.yaml` still wins, with the same warning), and `lodev start` warns when
`config.yaml` itself holds one. lodev never edits `config.yaml` to move it. `lodev config-global --tld` also moves the
`domains` in override files.

`nodejs`, `php_extensions`, `packages` and non-example `Dockerfile*` files in `.lodev/web-images/`
make lodev build a project image `lodev/<name>-web` on top of the PHP image. Projects without
any of them use the shared PHP image and build nothing. The image is rebuilt on the next
`lodev start` / `lodev restart` after one of them changes; `lodev start --rebuild` forces it. apt and
Node downloads are kept in BuildKit cache mounts, so a layer that reruns (e.g. after the PHP image
changed) installs without downloading again; `--rebuild` uses no cache at all, so it downloads.
