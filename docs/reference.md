# lodev reference

A lightweight, DDEV-like CLI for local PHP development on macOS with Docker.

Unlike DDEV, the heavy services — MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin (and opt-in MariaDB, RabbitMQ, MongoDB + Mongo Express) — run **once,
shared by all projects**. Each project only runs one small PHP container (php-fpm + nginx or
apache), so ten projects cost about the RAM of ten php-fpm processes, not ten databases.

## Install

Requirements: macOS or Linux with Docker (Docker Desktop, OrbStack, …). Windows binaries are built
but not supported at runtime: lodev edits `/etc/hosts` with sudo and checks ports with `lsof`.
[mkcert](https://github.com/FiloSottile/mkcert) is optional (`brew install mkcert`); without it
lodev runs mkcert in Docker and tells you how to trust its CA.

Every method below also installs shell completions: the install script and `make install`
for your `$SHELL` only, the packages for bash, zsh and fish.

### Install script (macOS, Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | sh
```

It picks the archive for your OS and CPU (x86_64 / arm64), verifies its sha256 against the
release's `checksums.txt`, and installs `lodev` plus completions under `/usr/local` (sudo only
if needed). Options: `VERSION=v1.2.3` (default: latest), `PREFIX` (default: `/usr/local`):

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | PREFIX="$HOME/.local" sh
```

It installs the completion for your `$SHELL` only (bash, zsh or fish). With a custom `PREFIX`
the script prints the lines to add for your `PATH` and for that completion, since shells only
read completions from the standard locations.

On macOS it also clears `com.apple.quarantine`, so Gatekeeper does not block lodev. `curl`,
Homebrew and `lodev self-upgrade` never set it; only a browser download does. If you
unpacked an archive downloaded with a browser yourself, run
`xattr -d com.apple.quarantine /path/to/lodev` once.

### Pre-release (nightly)

Every push to `main` rebuilds the `nightly` pre-release. To try it:

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | VERSION=nightly sh
```

or `lodev self-upgrade --version nightly`. A nightly build reports its `git describe` version
(e.g. `v0.1.1-3-g238f647`), shows no new-version notice, and `lodev self-upgrade` keeps it on nightly.
Go back to stable with `lodev self-upgrade --version v1.2.3`. Nightly has archives only (no `.deb`,
Homebrew or AUR).

### Homebrew (macOS, Linux)

```bash
brew install namnh198/tap/lodev
```

### Debian / Ubuntu (apt)

Download `lodev_<version>_amd64.deb` (or `_arm64.deb`) from the
[releases](https://github.com/namnh198/lodev/releases), then:

```bash
sudo apt install ./lodev_<version>_amd64.deb
```

### Arch Linux (AUR)

```bash
yay -S lodev-bin        # or paru -S lodev-bin
```

### Nix

```bash
nix profile add github:namnh198/lodev
nix run github:namnh198/lodev -- version
```

Both need flakes. Without `experimental-features = nix-command flakes` in `~/.config/nix/nix.conf`
(or `/etc/nix/nix.conf`), add `--extra-experimental-features 'nix-command flakes'` after `nix`.

`flake.nix` builds from source. Whenever `go.mod` changes, its `vendorHash` must follow: run
`nix build`, copy the `got: sha256-…` value from the error into `vendorHash`, and commit it.

### From source

```bash
make && sudo make install                  # build as yourself, install to /usr/local
make && make install PREFIX=~/.local       # no sudo
```

`make` writes the completion script for your `$SHELL` only; pick others with
`make COMPLETION_SHELLS="bash zsh fish"`.

`make` targets: `build` (all of linux/darwin/windows × amd64/arm64 into
`dist/bin/<os>_<arch>/`), `completions` (default; `build` plus the completion script for `$SHELL`), `dist` (archives + `checksums.txt`), `deb`, `packaging`,
`packages` (all of these), `install`, `uninstall`, `test`, `clean`. The version comes from
`git describe`; override with `make VERSION=v1.2.3`.

### Releasing

CI (`.github/workflows/ci.yml`) runs `go vet`, `go test` and `make build` on pushes to `main` and
on pull requests. Pushing a tag releases:

```bash
git tag v1.2.3 && git push origin v1.2.3
```

`.github/workflows/release.yml` runs `make packages` and creates the GitHub release with the
archives, `.deb` files, `checksums.txt`, `lodev.rb`, `PKGBUILD` and `.SRCINFO` (a tag with `-`,
like `v1.3.0-rc1`, is marked pre-release). A stable tag also commits `lodev.rb` to
`Formula/lodev.rb` in `namnh198/homebrew-tap` ("feat: release lodev vX.Y.Z"); that needs a repository
secret `HOMEBREW_TAP_TOKEN`, a token with Contents read/write on the tap repo. The rest is manual:
`PKGBUILD` + `.SRCINFO` to the `lodev-bin` AUR repo; bump `version` in `flake.nix`.

A stable tag also builds the PHP image for amd64 and arm64 (native runners) and pushes it to
`ghcr.io/namnh198/lodev-webserver:<tag>`; that lodev version pulls it instead of building it.
After the first push, make the package public once (GitHub → Packages → `lodev-webserver` →
Package settings → Change visibility → Public); until then lodev's pull fails and it builds locally.

`.github/workflows/nightly.yml` replaces the `nightly` pre-release on every push to `main`
(`make dist VER=nightly CHANNEL=nightly`); it is never marked latest, so stable users are not affected.

#### Signed releases

Both workflows sign `dist/checksums.txt` with an ed25519 key (`go run ./tools/sign -key-env
LODEV_SIGNING_KEY dist/checksums.txt`) and upload the result as `checksums.txt.sig`; a missing
`LODEV_SIGNING_KEY` secret fails the job, so no release goes out unsigned. `lodev self-upgrade`
checks that signature against the public key built into lodev (`releasePublicKey` in
`internal/update/download.go`) before it trusts `checksums.txt`, and installs nothing if it is
missing or wrong. That includes `nightly` and every older tag: releases made before signing
cannot be installed with `self-upgrade`; use `install.sh` or your package manager for them.

One-time setup, and key rotation (same steps):

1. `go run ./tools/sign -genkey` prints `private:` and `public:`.
2. Store the private key as the repository secret `LODEV_SIGNING_KEY`; keep it nowhere else.
3. Paste the public key into `releasePublicKey` and commit it, before pushing `main` (nightly
   signs on every push) and before tagging a release.

Rotating the key means a new lodev release: a build only trusts the key it shipped with, so
users on an older build must upgrade with `install.sh` or their package manager (and releases
signed with the old key stop verifying in new builds).

## Quick start

The first `lodev` command (any but `help`, `version`, `completion`, `self-upgrade`) sets lodev up in
`~/.lodev`: `config.yaml` with the defaults, `projects.json`, `data/`, the pinned `docker-compose` and
`docker-buildx` in `bin/`, and, without mkcert and with Docker running, the local CA in `rootCA/`.
Commands that need Docker (`start`, `stop`, `ssh`, `services …`, the shortcuts, …) stop with an
error when it is not running.

```bash
lodev service start          # Traefik, MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin
cd ~/Sites/myshop
lodev create                 # detects Laravel / WordPress / Magento / plain PHP
lodev start                # → https://myshop.test
lodev start myshop         # same, from any folder (also stop/restart/remove/ssh)
lodev magento setup:upgrade
```

`start` asks for your password once to add `.test` names to `/etc/hosts`.

Shared tools: Traefik dashboard <http://localhost:10999>, <https://phpmyadmin.test>,
<https://mailpit.test>, <https://rabbitmq.test> (guest/guest) once RabbitMQ is enabled, and
<https://mongo.test> once Mongo Express is enabled (MongoDB is `root/root`; web containers get `MONGODB_URI`).
All ports listen on `127.0.0.1` only.

Inside a project container: `DB_HOST=mysql` (`mariadb` for a project that uses MariaDB), `DB_PORT=3306`, `DB_DATABASE=<name>`,
`DB_USERNAME=root`, `DB_PASSWORD=root`, `REDIS_HOST=redis`, `OPENSEARCH_HOST=opensearch`,
`MAIL_HOST=mailpit`, `MAIL_PORT=1025`. PHP `mail()` goes to Mailpit.

## PHP image

All projects share one image, `lodev/webserver:<lodev version>` (e.g. `lodev/webserver:v0.1.0`),
with PHP 8.2–8.5, Composer, Node.js (`node_version`, default 24) with pnpm, Yarn 1.x and
grunt-cli, wp-cli, n98-magerun2, nano, vim-tiny and sqlite3. For native extensions and the
services: gcc, g++, make, autoconf, pkg-config, `phpize` and `pecl` (`php<ver>-dev` is installed),
and the `mysql` and `redis-cli` clients. A stable lodev pulls it from `ghcr.io/namnh198/lodev-webserver:<version>` on the
first `lodev start`; nightly and dev builds, a `node_version` other than 24, `--rebuild-base` or a
failed pull build it locally instead (about a minute). PHP 7.4, 8.0 and
8.1 add a small image (`lodev/webserver:<lodev version>-legacy-<ver>`) on top of it the first
time a project uses them.

- Change a project's PHP version: `lodev config --php-version=8.4` (or edit `php:` in `.lodev/config.yaml`), then `lodev restart`.
- After upgrading lodev, the first `lodev start` builds the images for the new version.
  `lodev start --rebuild` rebuilds the project image without the build cache; `--rebuild-base` also
  rebuilds the shared PHP image(s) from scratch (fresh PHP packages; slow).
- Images of older lodev versions are kept. List them with `docker images lodev/webserver`
  and free the space with `docker rmi lodev/webserver:<old version>`; lodev before
  `lodev/webserver` used `lodev/php` (`docker rmi $(docker images -q lodev/php)`).

## Global config — `~/.lodev/config.yaml`

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
mount_volume: false               # true = shared service data in <data_dir>/<service>, not Docker volumes
mount_paths:                      # optional, per service: mysql mariadb redis opensearch rabbitmq mongodb
  mysql: /Volumes/SSD/mysql
```

### Project homes and data folders

The container's home (`/home/<user>`) is rebuilt every time the container is created: from the
image's `/etc/skel` (a bashrc with a 10000-line history, `.gitconfig`, a `.bash_aliases` with `ll`), a generated
`~/.my.cnf`, then the files of `~/.lodev/homeadditional` and last those of the project's
`.lodev/homeadditional` (later ones win; nothing is deleted from the home, and the copies are
repeated on every container start; a file the user cannot read is skipped with a warning). Both
folders are mounted read-only. Put composer's auth at
`~/.lodev/homeadditional/.config/composer/auth.json` and your own `.gitconfig` in
`~/.lodev/homeadditional`, which lodev creates; the project's folder is committed with the project,
so keep secrets out of it. `lodev create` scaffolds `.lodev/homeadditional/.bash_aliases.example`
(rename it to `.bash_aliases`; `*.example` files there are not copied). It is in the [trust list](#commands-from-the-projects-repo), and the
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

`lodev config-global --mount-volume=true` (and optionally `--mysql-mount-path=<path>`, likewise
for the other data services) then `lodev service start` keeps the shared services' data in folders.
On that first `service start`, a service whose folder is empty gets its old `lodev-<service>` volume
copied in once (the service is stopped first; the volume is kept as a backup). If a copy fails,
empty the folder before retrying. Turning `mount_volume` off goes back to the volumes; data
written to the folders meanwhile is not copied back. OpenSearch's folder is made world-writable
(it runs as UID 1000 and does not take ownership of its data folder).

### Docker Compose and Buildx

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
`mount_volume`), kept across
`poweroff` and `services down`. Older lodev versions left an anonymous volume behind each time
Redis was recreated; list them with `docker volume ls -f dangling=true` and, once you have
checked that nothing else needs them, remove them with `docker volume prune` (it removes every
unused anonymous volume, not only lodev's).

## Project config — `.lodev/config.yaml`

Change any key but `name` with `lodev config` (e.g. `--php-version=8.4 --nodejs-version=24`,
`--add-services=redis`, `--nodejs-version=""` to remove a key); it keeps your comments and
saves nothing when the result is invalid. `lodev config` alone prints the current values.

```yaml
name: myshop                 # a-z 0-9 -; database name uses _ instead of -
type: magento                # laravel | wordpress | magento | php
php: "8.4"                   # 7.4 8.0 8.1 8.2 8.3 8.4 8.5
webserver: nginx             # nginx | apache | none (CLI-only, no domain)
docroot: pub
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

### Local overrides — `.lodev/config.*.yaml`

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
when `config.yaml` or another override still says otherwise), and `lodev start` warns when
`config.yaml` itself holds one. lodev never edits `config.yaml` to move it. `lodev config-global --tld` also moves the
`domains` in override files.

`nodejs`, `php_extensions`, `packages` and non-example `Dockerfile*` files in `.lodev/web-images/`
make lodev build a project image `lodev/<name>-web` on top of the PHP image. Projects without
any of them use the shared PHP image and build nothing. The image is rebuilt on the next
`lodev start` / `lodev restart` after one of them changes; `lodev start --rebuild` forces it. apt and
Node downloads are kept in BuildKit cache mounts, so a layer that reruns (e.g. after the PHP image
changed) installs without downloading again; `--rebuild` uses no cache at all, so it downloads.

## Cron

`cron: true` runs cron jobs in the web container with [supercronic](https://github.com/aptible/supercronic),
as your user, in `working_dir`, with the container's environment. Without `cron_jobs` the
project type picks the job:

| type | default job |
|---|---|
| magento | `* * * * * php bin/magento cron:run 2>&1 \| grep -v "Ran jobs by schedule" >> var/log/magento.cron.log \|\| true` |
| laravel | `* * * * * php artisan schedule:run >> storage/logs/schedule.log 2>&1` |
| wordpress | `* * * * * wp cron event run --due-now >> wp-content/cron.log 2>&1` |
| php | none: set `cron_jobs` |

Turn it on with `lodev config --cron=true` (edit `cron_jobs` in the file), then `lodev restart`.
`lodev start` writes the jobs to `.lodev/crontab` (generated, git-ignored); `lodev logs` shows
when each job runs.

## Daemons

`web_extra_daemons` runs long-lived dev processes (a Vite dev server, a queue worker) in the web
container under supervisor, as your user, in `directory` (default `working_dir`). A crashed
process is retried with a growing backoff, up to 100 times (`lodev restart` starts it afresh). `lodev start` writes `.lodev/daemons/` (generated, git-ignored); output
appears in `lodev logs`. Daemons are not health-checked, so a crashing one never fails
`lodev start`. Changes apply on `lodev restart`.

## Exposed ports

`web_extra_exposed_ports` makes a server inside the web container (a Vite dev server, say)
reachable as `https://<name>.<first domain>` (`https://vite.shop.test`), straight to the web
container even with Varnish. The host joins the certificate, `/etc/hosts` and the domain checks
like any domain; it needs a webserver and cannot equal one of `domains`. The web container gets
`LODEV_EXPOSED_<NAME>_URL` (name upper-cased, `-` becomes `_`) with the URL. Changes apply on
`lodev restart`.

Vite with a daemon and an exposed port:

```yaml
web_extra_daemons:
  - name: vite
    command: npm run dev -- --host 0.0.0.0 --port 5173 --strictPort
web_extra_exposed_ports:
  - name: vite
    container_port: 5173
```

```js
// vite.config.js
const url = process.env.LODEV_EXPOSED_VITE_URL; // https://vite.shop.test
export default { server: {
  host: '0.0.0.0', port: 5173, strictPort: true,
  origin: url, allowedHosts: [new URL(url).hostname],
  cors: { origin: /^https:\/\/([a-z0-9-]+\.)*shop\.test$/ },
  hmr: { host: new URL(url).hostname, protocol: 'wss', clientPort: 443 },
}};
```

## Environment variables

The web container gets lodev's variables (`DB_HOST`, `REDIS_HOST`, …), then `environment:` from
`.lodev/config.yaml`, then `.lodev/.env`; a later one wins, so either can override `DB_HOST`.
`LODEV_*` names are reserved. `.lodev/.env` is local (git-ignored, mode 0600), for secrets:

```bash
lodev dotenv set --app-key=abc --mage-run-code=default   # APP_KEY=abc, MAGE_RUN_CODE=default
lodev dotenv get --app-key                                # abc
```

It is a plain `KEY=value` file (comments, quotes and `export` allowed) you can also edit by hand.
Changes apply on `lodev restart`; cron jobs, `lodev ssh` and `lodev exec` see them too.

## Hooks

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

## Commands from the project's repo

A cloned repo can make lodev run commands: hooks (config steps, the scripts they name and the
`.lodev/hooks/<event>.d/` files), cron lines, `web_extra_daemons`, extra `.lodev/*.compose.yml`
files, `.lodev/web-images/` parts, `.lodev/homeadditional/` files (a symlinked folder is
followed; not `.example` ones), the project's `environment:` (from `config.yaml` and its overrides, not `.lodev/.env`),
and your own apache config: the files in `.lodev/apache/` (not `.example` ones) and
`.lodev/apache-webserver.conf` once it no longer carries lodev's marker. lodev lists them and asks
`Run these commands? [y/N]` once, on `lodev create` (a folder with a `.lodev/config.yaml`, or one
whose repo already has `.lodev` files; nothing to run asks nothing). A declined question registers
nothing. Where lodev cannot ask (no terminal) it fails with the list; pass `--yes` to `create` to
approve it. Nothing is stored: `start`, `restart` and the hooks run by `stop`, `db-import` and
`snapshot restore` do not ask again.

Accepted risk: a `git pull` that adds or changes a hook, daemon, compose file, web-image part,
homeadditional or apache file runs at the next start without a question. Review what a pull
brings into `.lodev/` before starting the project.

## Domains and TLD

Project domains (`<name>.test`) and the shared services' (`mailpit.test`, `phpmyadmin.test`,
`rabbitmq.test`, `mongo.test`) end in `tld` from `~/.lodev/config.yaml`: `test` by default, `local`
for a fresh install on Linux. Switch every project at once:

```bash
lodev config-global --tld=local
```

It rewrites each project's `domains` in `.lodev/config.yaml` (shop.test → shop.local, comments
kept) and `projects.json`, replaces lodev's blocks in `/etc/hosts`, then runs `service start` and
restarts the projects that are running; stopped ones pick the new domains up on their next start.
Nothing is written if any project cannot move. On macOS `.local` goes through mDNS (slow, may skip
`/etc/hosts`), so `test` or `localhost` work better there. After editing `tld` in the file by hand,
run `lodev config-global --tld=<new> --from-tld=<old>`.

## Performance mode

With `performance_mode: bind` (the default on macOS, where bind mounts are slow; Linux defaults to `none`),
some project folders live in Docker volumes instead of the bind mount. Set it per project
(`lodev config --performance-mode=bind|none`) or for all (`lodev config-global --performance-mode=...`);
`""` goes back to the next level (project, then global, then the OS default).

| Type | Folders in volumes |
|---|---|
| magento | `var`, `generated`, `node_modules`* |
| laravel, wordpress, php | `node_modules`* |

\* only when `package.json` exists at the project root.

Laravel's `storage` stays on the bind mount (it holds real data); add it to `volume_dirs` if you want it
in a volume. `volume_dirs` adds your own folders (`lodev config --add-volume-dirs=pub/media`): relative
paths of `A-Z a-z 0-9 . _ -` segments, not `.lodev`, no folder inside another one in volumes, and no two
that map to the same volume name (`lodev-<name>-<folder with / as ->`).

- `var`, `generated` and `node_modules` start empty in their volume. Every `volume_dirs` folder is copied
  from the host once, when its volume is created.
- The host no longer sees what the container has in these folders: read logs with `lodev logs` or
  `lodev exec tail`, and host-side `npm` or IDE indexing of `generated/` need `performance_mode: none`.
- `lodev start` creates the volumes (labelled `lodev.project=<name>`) before the containers and prints
  what each folder got. A folder that is a symlink on the host is skipped with a warning.
- Files the container writes into a seeded folder (e.g. uploads in `pub/media`) live only in its volume.
  `lodev remove` keeps the volumes and lists them; `lodev remove --volumes` deletes them after asking.
  Switching to `none` or dropping a folder from `volume_dirs` also leaves the volume: `start` lists
  those as unused.
- `lodev describe` shows the effective mode and folders.

## Varnish

`lodev config --varnish=true` (optional `--varnish-version=9.1`) then `lodev restart` puts a
`lodev-<name>-varnish` container in front of the web server: Traefik → varnish:80 → web:80,
and the web container gets `VARNISH_HOST=lodev-<name>-varnish`. Not available with `webserver: none`.
`--varnish=false` removes the container on the next start.

lodev writes a default VCL to `.lodev/varnish.vcl` (backend `lodev-<name>-web:80`, PURGE/BAN from the Docker
networks, no caching of non-GET/HEAD or authorized requests). Delete its first line to manage it
yourself. For Magento, use Magento's VCL and point its cache at Varnish:

```bash
bin/magento varnish:vcl:generate --backend-host=lodev-<name>-web --backend-port=80 \
  --access-list=lodev-<name>-web > .lodev/varnish.vcl
bin/magento setup:config:set --http-cache-hosts=lodev-<name>-varnish:80
bin/magento config:set system/full_page_cache/caching_application 2
```

## User and permissions

`lodev ssh` and `lodev exec` run in the web container as your user (`user:`, default your host
username; lowercased, characters outside `a-z 0-9 _ -` become `_`, a leading digit or `-` gets a
`_` prefix, `root`/`www-data` get `_user`, max 32 chars). `lodev start` / `restart` print one warning when
the name had to change. The user has passwordless `sudo`, the hostname is `<name>-web` and the
workdir is `working_dir` (default `/app`, as on Magento Cloud). Projects started by an older lodev ran in
`/var/www/html`: the first start after the change warns, so you can flush caches that hold paths and
update the IDE's path mapping. nginx / apache / php-fpm workers run as that user too, so files PHP
writes are owned by you on the host. `xdebug` toggling still runs as root.

After upgrading, the first `lodev start` rebuilds `lodev/webserver` (the base image no longer bakes
the host UID into `www-data`). Run `lodev restart` in every project that was running before the
upgrade, or `ssh`/`exec` won't find your user.

`-s`/`--service` picks another service of the project (default `web`): `lodev ssh -s redis-commander`,
`lodev exec -s redis-commander ls`. Those use the image's default user and directory, `bash` if the
image has it, else `sh`. For `exec` the flag only counts before the command.

## Customizing the web image

Put `Dockerfile*` files in `.lodev/web-images/` (added in name order after the generated
steps; `*.example` files are ignored). Plain Dockerfile instructions, no `FROM`:

```dockerfile
# .lodev/web-images/Dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends poppler-utils && rm -rf /var/lib/apt/lists/*
```

Editing a file rebuilds the project image on the next `lodev start` / `restart`; leaving it
alone builds nothing. `lodev create` scaffolds `Dockerfile.example`.

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
it is ambiguous); `web` works only on the project's `default` network. `lodev create` scaffolds `example.compose.yml.example`.

An extra service gets an HTTPS route by setting `VIRTUAL_HOST` in its `environment:` (one host, or
a comma-separated list, each ending in your TLD) and `VIRTUAL_PORT` (the container port, default 80).
It needs `networks: [lodev, default]`: Traefik reaches it on `lodev`, as `lodev-<project>-<service>-1`
(or its `container_name`).

```yaml
# .lodev/adminer.compose.yml
services:
  adminer:
    image: adminer
    networks: [lodev, default]
    environment:
      VIRTUAL_HOST: adminer.test          # one host, or several: a.test,b.test
      VIRTUAL_PORT: "8080"                # container port; default 80
```

`lodev start` adds these hosts to the project's certificate, `/etc/hosts` and the web container's
`extra_hosts`, and fails if another project or a shared service already uses one. It writes the
project's routes to `~/.lodev/traefik/p-<project>.yaml`; `stop`, `remove` and `poweroff` delete
it. `traefik.*` labels are ignored (Traefik reads route files, not the Docker socket):
use `VIRTUAL_HOST` / `VIRTUAL_PORT` instead.

## Customizing the shared services

MySQL and MariaDB read `~/.lodev/services/mysql/lodev.cnf` and `~/.lodev/services/mariadb/lodev.cnf` (mounted as
`/etc/mysql/conf.d/zz-lodev.cnf`). `lodev service start` writes them when missing, with lodev's
defaults (`sql_mode` without `ONLY_FULL_GROUP_BY`, `max_allowed_packet=500M`, 8-hour timeouts,
`mysql_native_password` on MySQL 8.0), and then leaves your edits alone. Apply an edit with
`docker restart lodev-mysql` (or `lodev-mariadb`).

Put compose files in `~/.lodev/services/` named `*.compose.yml` (or `.yaml`); `lodev service start`
merges them, in name order, over the generated `services.compose-base.yaml` into `services.compose-full.yaml`, which
every `lodev service` command then uses. A file that does not merge makes `service start` fail
and keeps the last good `services.compose-full.yaml`. `service start` writes `example.compose.yml.example`
there (never overwritten). For example, `~/.lodev/services/opensearch.compose.yml`:

```yaml
services:
  opensearch:
    mem_limit: 3g
```

A service in those files gets an HTTPS route by setting `VIRTUAL_HOST` (comma-separated, ending in your
TLD) and, when it does not listen on port 80, `VIRTUAL_PORT` in its `environment:`; it also needs
`networks: [lodev, default]`. `lodev service start` writes the routes of the running services to
`~/.lodev/traefik/lodev-services.yaml`, adds the hosts to the certificate and `/etc/hosts`, and fails if a
project already uses one. `traefik.*` labels are ignored (Traefik no longer reads the Docker socket).

```yaml
services:
  adminer:
    image: adminer
    networks: [lodev, default]
    environment: {VIRTUAL_HOST: adminer.test, VIRTUAL_PORT: "8080"}
```

lodev owns `certs.yaml`, `lodev-services.yaml` and `p-*.yaml` in `~/.lodev/traefik/`. Any other `*.yaml`
there is yours (middlewares, path rules): Traefik loads it, lodev never writes or deletes it.

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

## Health checks

Every container has a Docker healthcheck, and `lodev start` / `lodev service start` wait for it (up to
`container_timeout`, default 120s). They stop at once when a container exits, keeps restarting or turns
unhealthy, and print the last check and where to look:

```
lodev-shop-web is unhealthy: /app:OK nginx:OK php-fpm:FAILED phpstatus:FAILED
Check:
  lodev logs -s web
  docker logs lodev-shop-web
  docker inspect --format "{{ json .State.Health }}" lodev-shop-web
```

The web container's `lodev-healthcheck` checks the project mount (`.lodev/config.yaml` readable), the
webserver and php-fpm, and PHP through the webserver (`curl http://127.0.0.1/phpstatus`, served on
127.0.0.1 whatever the project's webserver config; other paths on 127.0.0.1 still reach the project).
With `webserver: none` it checks the mount and the PHP CLI. Shared services use their own check
(`SHOW DATABASES`, `redis-cli ping`, OpenSearch `_cluster/health`, `mailpit readyz`, …).

## Commands

| Command | What it does |
|---|---|
| `lodev service start\|stop\|status\|logs [svc]` | Manage the shared stack (aliases `services`, `svc`; `up`/`down` for start/stop). `stop --volumes` deletes all data (asks first) |
| `lodev service enable\|disable <svc>...` | Turn shared services on/off, several at once (data is kept) |
| `lodev create [--name --type --php --webserver --docroot --no-ask --yes]` | Create `.lodev/config.yaml` (alias `init`). On a terminal an interactive form asks name/type/PHP/docroot/webserver (detected values preselected); `--no-ask` skips it; `--docroot .` = project root. It also registers the project in `~/.lodev/projects.json`; in a folder that already has a `.lodev/config.yaml` (a cloned project, or after `lodev remove`) it warns and registers that config as it is; either way it first asks about the [commands the repo runs](#commands-from-the-projects-repo) (`--yes` approves them) |
| `lodev dotenv set --key=value [...]` / `lodev dotenv get --key` | Set or print variables in `.lodev/.env` (`--app-key` is `APP_KEY`); apply with `lodev restart` |
| `lodev config [flags]` | Show or change `.lodev/config.yaml` (e.g. `--php-version=8.4 --nodejs-version=24`, `--add-services=redis`); validated before saving, keeps comments; the local keys `--enable-xdebug` and `--opcache-validate-timestamps` go to `.lodev/config.local.yaml` instead; apply with `lodev restart` |
| `lodev config-global [flags]` | Show or change `~/.lodev/config.yaml` (e.g. `--mysql-version=8.4`, `--enable-services=rabbitmq`, `--auto-create-db=false`); apply with `lodev service start` |
| `lodev start [--rebuild \| --rebuild-base \| --all]` | Start the project (alias `up`); `--rebuild` rebuilds the project image without cache, `--rebuild-base` the PHP image(s) too; `--all` starts every known project from any folder. `start` and `restart` need a project made by `lodev create` (a `.lodev/config.yaml` registered in `~/.lodev/projects.json`) |
| `lodev stop` / `lodev restart [--all]` | Stop / restart the project container; `restart --all` restarts every known project |
| `lodev remove [--db] [--volumes] [--yes]` | Remove container, hosts entries, cert (alias `down`); `--db` drops the database; `--volumes` deletes the performance_mode volumes (they are kept and listed otherwise) |
| `lodev list` | All projects with status, type, domains and path (`~` for your home) as a table (alias `ps`) |
| `lodev poweroff` | Remove every project's containers and networks and take the shared services down (data, project images, hosts entries and certificates are kept) |
| `lodev db-export [db \| -n db] [-s svc] [-f file] [-c gz] [-e 'options']` | Dump a database of the shared `mysql`, `mariadb` or `mongodb` (default: the project's database and service) to `<db>-<time>.sql[.gz]` (`.archive` for MongoDB), or `-f -` for stdout. `-e` adds options to mysqldump / mariadb-dump / mongodump, split like a shell (quotes work, repeatable), e.g. `lodev db-export -c gz -f dump.sql.gz -e '--skip-add-locks'` |
| `lodev db-import [db \| -n db] -f file [-s svc] [-e 'options']` | Load a dump into a database (created if missing; plain or gzip, detected); replaces its tables/collections; `-f -` reads stdin. MongoDB dumps can land in another database name. Into a Magento project's own database it then sets the base URLs on every scope to `https://<domain>/`, clears CDN static/media URLs and the cookie domain, and flushes the cache (`--no-magento` skips it); `--no-hooks` skips `post-db-import`. `-e` adds options to mysql / mariadb / mongorestore, like db-export's |
| `lodev snapshot create\|list\|restore\|delete [name] [-p project]` | Save the project's databases (its MySQL/MariaDB one, plus MongoDB) to `~/.lodev/snapshots/<project>/<name>/`, list them, load one back (default: the newest; asks first, `--yes` skips; runs `post-snapshot-restore`), or delete one |
| `lodev logs [-s service] [-j]` | Follow a service's logs (default web; `-s mysql` for a shared service); `-j` pretty-prints JSON lines with jq (web only) |
| `lodev ssh [name] [-s service]` / `lodev exec [-s service] <cmd>` | Shell / command in the web container, or in another service of the project |
| `lodev php\|composer\|npm\|node\|pnpm\|yarn\|grunt\|artisan\|wp\|magento\|n98 ...` | Shortcuts (`n98` runs `n98-magerun2`) |
| `lodev xdebug on\|off\|toggle\|status` | Turn Xdebug on/off, or flip it with `toggle` (port 9003); saved as `enable_xdebug` in `.lodev/config.local.yaml` (never the committed `config.yaml`), so `lodev start` / `restart` keep it (a stopped project just gets the setting) |
| `lodev describe [project]` | Project URLs, its services with status, in-network/host ports, images and credentials, plus phpMyAdmin/Mailpit (aliases `status`, `st`, `desc`) |
| `lodev open [project\|phpmyadmin\|mailpit\|rabbitmq\|mongo-express\|traefik] [--print]` | Open the project here (or another one, or a shared service page) in the browser; `--print` only prints the URL |
| `lodev clean [--dry-run] [--yes] [--keep-cache]` | Remove lodev images no container uses (older versions, `lodev/php`, removed projects, and `ghcr.io/namnh198/lodev-webserver` images: older ones on a stable release, all of them on nightly, which builds its own) and Docker build cache older than 7 days; lists them and asks first; never touches volumes |
| `lodev doctor` | Check Docker, the pinned compose/buildx, the global config, ports, the local CA, `/etc/hosts`, the shared services, the projects, userns-remap and Docker disk use; each row says how to fix it; exits 1 on a problem |
| `lodev self-upgrade [--version vX.Y.Z]` | Replace lodev (and the completions installed next to it) with the latest release, or the given one; verifies the release's signature and sha256 (unsigned releases are refused) and asks for your sudo password when the folder needs root. Homebrew, apt, AUR and Nix installs are upgraded with their package manager instead |
| `lodev version` | lodev, Docker, Docker Compose and Docker Buildx versions, OS and architecture; `lodev -v` prints only `lodev version <v>` |
| `lodev --debug <cmd>` | Also print every docker command lodev runs (or set `LODEV_DEBUG=1`); set `NO_COLOR=1` to turn colors off |

**Known limit:** a running project learns about domains of projects started after it on its
next `lodev start` / `lodev restart`.

## License

MIT, see [LICENSE](../LICENSE).
