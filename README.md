# lodev

A lightweight, DDEV-like CLI for local PHP development on macOS with Docker.

Unlike DDEV, the heavy services — MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin (and opt-in MariaDB, RabbitMQ, MongoDB + Mongo Express) — run **once,
shared by all projects**. Each project only runs one small PHP container (php-fpm + nginx or
apache), so ten projects cost about the RAM of ten php-fpm processes, not ten databases.

## Install

Requirements: macOS or Linux with Docker (Docker Desktop, OrbStack, …). Windows binaries are built
but not supported at runtime: lodev edits `/etc/hosts` with sudo and checks ports with `lsof`.
[mkcert](https://github.com/FiloSottile/mkcert) is optional (`brew install mkcert`); without it
lodev runs mkcert in Docker and tells you how to trust its CA.

Every method below also installs bash, zsh and fish completions.

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

With a custom `PREFIX` the script prints the lines to add for your `PATH` and for bash, zsh
and fish completions, since shells only read completions from the standard locations.

On macOS it also clears `com.apple.quarantine`, so Gatekeeper does not block lodev. `curl`,
Homebrew and `lodev self-upgrade` never set it; only a browser download does. If you
unpacked an archive downloaded with a browser yourself, run
`xattr -d com.apple.quarantine /path/to/lodev` once.

### Pre-release (edge)

Every push to `main` rebuilds the `edge` pre-release. To try it:

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | VERSION=edge sh
```

or `lodev self-upgrade --version edge`. An edge build reports its `git describe` version
(e.g. `v0.1.1-3-g238f647`), shows no new-version notice, and `lodev self-upgrade` keeps it on edge.
Go back to stable with `lodev self-upgrade --version v1.2.3`. Edge has archives only (no `.deb`,
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
nix profile install github:namnh198/lodev
nix run github:namnh198/lodev -- version
```

`flake.nix` builds from source. Its `vendorHash` is a placeholder until the first build: run
`nix build`, copy the `got: sha256-…` value from the error into `vendorHash`, and commit it
(again whenever `go.mod` changes).

### From source

```bash
make && sudo make install                  # build as yourself, install to /usr/local
make && make install PREFIX=~/.local       # no sudo
```

`make` targets: `build` (default; all of linux/darwin/windows × amd64/arm64 into
`dist/bin/<os>_<arch>/`), `completions`, `dist` (archives + `checksums.txt`), `deb`, `packaging`,
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
like `v1.3.0-rc1`, is marked pre-release). Publishing is then manual: copy `lodev.rb` to
`Formula/` in the `namnh198/homebrew-tap` repo, and `PKGBUILD` + `.SRCINFO` to the `lodev-bin`
AUR repo; bump `version` in `flake.nix`.

`.github/workflows/edge.yml` replaces the `edge` pre-release on every push to `main`
(`make dist VER=edge CHANNEL=edge`); it is never marked latest, so stable users are not affected.

## Quick start

```bash
lodev services up          # Traefik, MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin
cd ~/Sites/myshop
lodev create                 # detects Laravel / WordPress / Magento / plain PHP
lodev start                # → https://myshop.test
lodev start myshop         # same, from any folder (also stop/restart/remove)
lodev magento setup:upgrade
```

`start` asks for your password once to add `.test` names to `/etc/hosts`.

Shared tools: Traefik dashboard <http://localhost:8080>, <https://phpmyadmin.test>,
<https://mailpit.test>, <https://rabbitmq.test> (guest/guest) once RabbitMQ is enabled, and
<https://mongo.test> once Mongo Express is enabled (MongoDB is `root/root`; web containers get `MONGODB_URI`).
All ports listen on `127.0.0.1` only.

Inside a project container: `DB_HOST=mysql` (`mariadb` for a project that uses MariaDB), `DB_PORT=3306`, `DB_DATABASE=<name>`,
`DB_USERNAME=root`, `DB_PASSWORD=root`, `REDIS_HOST=redis`, `OPENSEARCH_HOST=opensearch`,
`MAIL_HOST=mailpit`, `MAIL_PORT=1025`. PHP `mail()` goes to Mailpit.

## PHP image

All projects share one image, `lodev/php`, with PHP 8.2–8.5; it is built on the first
`lodev start` (about a minute). PHP 7.4, 8.0 and 8.1 add a small image
(`lodev/php:legacy-<ver>`) on top of it the first time a project uses them.

- Change a project's PHP version: `lodev config --php-version=8.4` (or edit `php:` in `.lodev/config.yaml`), then `lodev restart`.
- After upgrading lodev, `lodev start` rebuilds the images by itself when their files
  changed. `lodev start --rebuild` forces a clean rebuild (fresh PHP packages).
- Coming from an older lodev that built one image per version? Free the space with
  `docker rmi lodev/php:7.4 lodev/php:8.0 lodev/php:8.1 lodev/php:8.2 lodev/php:8.3 lodev/php:8.4 lodev/php:8.5`
  ("No such image" errors are fine).

## Global config — `~/.lodev/config.yaml`

All keys optional; defaults shown. Change them with `lodev config-global` (e.g. `--mysql-version=8.4`,
`--enable-services=rabbitmq`) or edit the file. Run `lodev services up` after editing: `lodev start` leaves
the shared services alone while everything it needs is running, so it does not apply these changes.

```yaml
ports:
  http: 80
  https: 443
  dashboard: 8080   # Traefik dashboard
  mysql: ""         # e.g. 3306 to reach MySQL from TablePlus etc.
  mariadb: ""       # e.g. 3307; must differ from mysql
  mongodb: ""       # e.g. 27017 to reach MongoDB from MongoDB Compass etc.
services:           # traefik is always on
  mysql: true
  redis: true
  opensearch: true
  mailpit: true
  phpmyadmin: true
  mariadb: false     # opt-in: `lodev services enable mariadb`, or `services: [mariadb]` in a project
  rabbitmq: false    # opt-in: `lodev services enable rabbitmq`, or `services: [rabbitmq]` in a project
  mongodb: false     # opt-in: `lodev services enable mongodb`, or `services: [mongodb]` in a project
  mongo-express: false # opt-in web UI at https://mongo.test; also starts mongodb
memory:
  opensearch: 2g    # container limit, 1g..4g; JVM heap is half
versions:           # 8 | 8.0 | 8.0.36; only these keys
  mysql: "8.0"
  mariadb: "11.4"
  redis: "7"        # redis:<v>-alpine
  opensearch: "2"
  rabbitmq: "4"     # rabbitmq:<v>-management
  mongodb: "8"      # mongo:<v>
update_check: true  # after a command, say when a newer lodev release exists (asks GitHub at most every 4h);
                    # turn off with `lodev config-global --update-check=false`
use_docker_compose_system: false  # true = the system `docker compose` instead of lodev's pinned one
use_docker_buildx_system: false   # true = the system `docker build` (buildx) instead of lodev's pinned one
data_dir: ~/.lodev/data           # project homes and mounted service data ("" = ~/.lodev/data)
mount_volume: false               # true = shared service data in <data_dir>/<service>, not Docker volumes
mount_paths:                      # optional, per service: mysql mariadb redis opensearch rabbitmq mongodb
  mysql: /Volumes/SSD/mysql
```

### Project homes and data folders

Each project's web container mounts `<data_dir>/home-<name>` as the user's home, so bash and
mysql history, composer auth (`~/.config/composer/auth.json`), the composer and npm caches and
anything else in the home survive container re-creates. lodev creates the folder on
`lodev start`; `lodev remove` keeps it (and says where). Paths are absolute or start with `~/`;
changing `data_dir` (`lodev config-global --data-dir=<path>`) moves nothing.

`lodev config-global --mount-volume=true` (and optionally `--mysql-mount-path=<path>`, likewise
for the other data services) then `lodev services up` keeps the shared services' data in folders.
On that first `services up`, a service whose folder is empty gets its old `lodev-<service>` volume
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
`lodev services up` warns when the image changes. Dump your databases first: a major
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
php: "8.3"                   # 7.4 8.0 8.1 8.2 8.3 8.4 8.5
webserver: nginx             # nginx | apache | none (CLI-only, no domain)
docroot: pub
domains:                     # default <name>.test; all must end in .test
  - myshop.test
  - store-en.test
services: [mysql, redis, opensearch]   # mariadb instead of mysql for MariaDB (not both)
nodejs: "20"                 # optional: lts | latest | current | 20 | 20.11 | 20.11.1 (installed with n)
php_extensions: [ldap]       # optional: a-z 0-9 _ ; installed for the project's PHP version
packages: [ffmpeg]           # optional: extra apt packages
user: kuroko                 # optional: shell user in the web container; default = your host username
varnish: true                # optional: Varnish in front of the web server (default false)
varnish_version: "7.6"       # optional: the varnish image tag (default 7.6)
```

`nodejs`, `php_extensions`, `packages` and non-example `Dockerfile*` files in `.lodev/web-images/`
make lodev build a project image `lodev/<name>-web` on top of the PHP image. Projects without
any of them use the shared PHP image and build nothing. The image is rebuilt on the next
`lodev start` / `lodev restart` after one of them changes; `lodev start --rebuild` forces it.

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
workdir `/var/www/html`. nginx / apache / php-fpm workers run as that user too, so files PHP
writes are owned by you on the host. `xdebug` toggling still runs as root.

After upgrading, the first `lodev start` rebuilds `lodev/php` (the base image no longer bakes
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

## Customizing the shared services

Put compose files in `~/.lodev/services/` named `*.compose.yml` (or `.yaml`); `lodev services up`
merges them, in name order, over the generated `compose.yaml` into `compose-full.yaml`, which
every `lodev services` command then uses. A file that does not merge makes `services up` fail
and keeps the last good `compose-full.yaml`. `services up` writes `example.compose.yml.example`
there (never overwritten). For example, `~/.lodev/services/opensearch.compose.yml`:

```yaml
services:
  opensearch:
    mem_limit: 3g
```

## PHP and webserver config

| Path | Effect |
|---|---|
| `.lodev/php/*.ini` | Extra php.ini settings (`memory_limit=4G`) |
| `.lodev/nginx/*.conf` or `.lodev/apache/*.conf` | Snippets included in the server / vhost block |
| `.lodev/nginx-webserver.conf`, `.lodev/apache-webserver.conf` | The whole webserver config (generated, one file per webserver) |

Apply with `lodev restart`; none of these builds an image. A generated webserver conf starts with
`# lodev-generated: delete this line to manage this file yourself`; delete that line to take the
file over (lodev stops regenerating it and it is no longer ignored). Switching `webserver:` does
not touch the other webserver's file. `lodev create` scaffolds `*.example` files in each folder.

lodev no longer writes your project's root `.gitignore`; old `.lodev/compose.yaml` /
`.lodev/webserver.conf` lines there can be removed (lodev keeps `.lodev/.gitignore` itself).

**Upgrading from an older lodev:** run `lodev start` once in each project before `stop`, `exec`
or `ssh` (they use the generated `lodev.compose-full.yml`).

## Commands

| Command | What it does |
|---|---|
| `lodev services up\|down\|status\|logs [svc]` | Manage the shared stack (aliases `service`, `svc`). `down --volumes` deletes all data (asks first) |
| `lodev services enable\|disable <svc>` | Turn a shared service on/off (data is kept) |
| `lodev create [--name --type --php --webserver --docroot --no-ask]` | Create `.lodev/config.yaml` (alias `init`). On a terminal an interactive form asks name/type/PHP/docroot/webserver (detected values preselected); `--no-ask` skips it; `--docroot .` = project root |
| `lodev config [flags]` | Show or change `.lodev/config.yaml` (e.g. `--php-version=8.4 --nodejs-version=24`, `--add-services=redis`); validated before saving, keeps comments; apply with `lodev restart` |
| `lodev config-global [flags]` | Show or change `~/.lodev/config.yaml` (e.g. `--mysql-version=8.4`, `--enable-services=rabbitmq`); apply with `lodev services up` |
| `lodev start [--rebuild \| --all]` | Start the project (alias `up`); `--rebuild` rebuilds the PHP image(s) and the project image; `--all` starts every known project from any folder |
| `lodev stop` / `lodev restart [--all]` | Stop / restart the project container; `restart --all` restarts every known project |
| `lodev remove [--db] [--yes]` | Remove container, hosts entries, cert (alias `down`); `--db` drops the database |
| `lodev list` | All projects with status, type, domains and path (`~` for your home) as a table (alias `ps`) |
| `lodev poweroff` | Remove every project's containers and networks and take the shared services down (data, project images, hosts entries and certificates are kept) |
| `lodev ssh [-s service]` / `lodev exec [-s service] <cmd>` | Shell / command in the web container, or in another service of the project |
| `lodev php\|composer\|npm\|node\|artisan\|wp\|magento ...` | Shortcuts |
| `lodev xdebug on\|off\|toggle\|status` | Turn Xdebug on/off, or flip it with `toggle` (port 9003) |
| `lodev describe [project]` | Project URLs, its services with status, in-network/host ports, images and credentials, plus phpMyAdmin/Mailpit (aliases `status`, `st`, `desc`) |
| `lodev self-upgrade [--version vX.Y.Z]` | Replace lodev (and the completions installed next to it) with the latest release, or the given one; checks the sha256 and asks for your sudo password when the folder needs root. Homebrew, apt, AUR and Nix installs are upgraded with their package manager instead |
| `lodev version` | lodev, Docker, Docker Compose and Docker Buildx versions, OS and architecture; `lodev -v` prints only `lodev version <v>` |
| `lodev --debug <cmd>` | Also print every docker command lodev runs (or set `LODEV_DEBUG=1`); set `NO_COLOR=1` to turn colors off |

**Known limit:** a running project learns about domains of projects started after it on its
next `lodev start` / `lodev restart`.

## License

MIT, see [LICENSE](LICENSE).
