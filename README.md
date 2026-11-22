<img src="docs/logo/lodev-logo.svg" alt="lodev" height="64">

The CLI tool for creating and managing local PHP development environments with Docker.

[![Install](https://img.shields.io/badge/install-555)](#getting-started)
[![Latest release](https://img.shields.io/github/v/release/namnh198/lodev)](https://github.com/namnh198/lodev/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/namnh198/lodev/total)](https://github.com/namnh198/lodev/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

## Getting Started

You need macOS or Linux with Docker (Docker Desktop, OrbStack, …).

### Bash script (Recommend)

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | sh
```

Options: `VERSION=v1.2.3` (default: latest), `PREFIX` (default: `/usr/local`):

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | PREFIX="$HOME/.local" sh
```

### Brew

```bash
brew install namnh198/tap/lodev
```

### Arch Linux

lodev is not on the AUR; each release ships the `PKGBUILD` instead:

```bash
mkdir lodev-bin && cd lodev-bin
curl -fLO https://github.com/namnh198/lodev/releases/latest/download/PKGBUILD
makepkg -si
```

Run the same commands again to upgrade.

### NixOS

```bash
nix profile add github:namnh198/lodev
```

### Ubuntu/Debian

Download the `.deb` for your CPU from the [latest release](https://github.com/namnh198/lodev/releases/latest)
and install it with apt (set `VERSION` to that release, without the `v`):

```bash
VERSION=0.1.11
curl -fLO https://github.com/namnh198/lodev/releases/download/v$VERSION/lodev_${VERSION}_$(dpkg --print-architecture).deb
sudo apt install ./lodev_${VERSION}_$(dpkg --print-architecture).deb
```

### Build from source

```bash
git clone https://github.com/namnh198/lodev.git
make && sudo make install # build as yourself, install to /usr/local
make && make install PREFIX=~/.local # no sudo
```

## Why LODEV?

- **One shared stack.** MySQL, Redis, OpenSearch and Mailpit run once for every project. Each project
  runs a single small PHP container (php-fpm with nginx or apache), so ten projects cost about ten
  php-fpm processes, not ten databases.
- **Ready for real apps.** It supports PHP 7.4–8.6, with Composer, Node.js, pnpm, Yarn, wp-cli and n98-magerun2 in the
  image. It detects the framework and sets up the database, HTTPS domains and mail capture for you.
- **Your config travels with the repo.** `.lodev/config.yaml` holds the project's settings, hooks,
  cron and extra services. Personal toggles such as Xdebug stay in the uncommitted `config.local.yaml`.

## Starting with new project

```bash
lodev service start      # Start the shared services (mysql, redis, opensearch, mailpit, phpmyadmin, traefik)
cd ~/Sites/myshop
lodev create             # detects magento / laravel / wordpress / nextjs / php
lodev start              # → https://myshop.test
```

How `https://myshop.test` reaches your project:

- **Names (no DNS server).** lodev writes each project's domains into its own block in `/etc/hosts`,
  pointing them at `127.0.0.1`; `lodev start` asks for your password when that file needs `sudo`, and
  `lodev remove` takes the block out again.
- **HTTPS.** Traefik serves every domain with a certificate made by
  [mkcert](https://github.com/FiloSottile/mkcert). With mkcert installed, lodev uses it and its CA is
  added to your trust store once. Without it, lodev runs mkcert in a container, keeps its own CA in
  `~/.lodev/rootCA`, and prints the one command that makes your system and browsers trust it.
- **Between projects.** The web containers trust the same CA and resolve every project's domains, so
  one project can call another over HTTPS (e.g. a Next.js front end querying `https://myshop.test/graphql`).

## Everyday Commands

| Command                                                   | What it does                                                                                   |
| --------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| `lodev start` / `stop` / `restart`                        | Start, stop or restart the current project (`start --all` / `restart --all` for every project) |
| `lodev list`                                              | All projects with status and domains                                                           |
| `lodev describe`                                          | URLs, services, ports and credentials of the project                                           |
| `lodev ssh` / `lodev exec <cmd>`                          | Shell or command in the project's web container                                                |
| `lodev composer` / `npm` / `artisan` / `wp` / `magento` … | Run the tool inside the container                                                              |
| `lodev db-import -f dump.sql` / `lodev db-export`         | Load or dump the project's database                                                            |
| `lodev snapshot create` / `restore`                       | Save and restore the project's databases                                                       |
| `lodev xdebug on` / `off`                                 | Toggle Xdebug (port 9003)                                                                      |
| `lodev open`                                              | Open the project in the browser                                                                |
| `lodev doctor`                                            | Check Docker, ports, certificates and services, with a fix for each problem                    |
| `lodev self-upgrade`                                      | Upgrade to the latest signed release                                                           |

Every command takes `--help`. The full docs live at <https://namnh198.github.io/lodev/>
(source: [docs/contents](docs/contents)): configuration, hooks, services and every command.

## License

MIT, see [LICENSE](LICENSE).
