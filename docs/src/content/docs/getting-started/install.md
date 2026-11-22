---
title: "Install"
description: "Install lodev on macOS or Linux: script, Homebrew, apt, Arch, Nix or from source."
sidebar:
  order: 1
---

Installs lodev, a DDEV-like CLI for local PHP development on macOS or Linux with Docker.

Unlike DDEV:

- The heavy services run once, shared by all projects: MySQL, Redis, OpenSearch, Mailpit and phpMyAdmin. MariaDB, RabbitMQ and MongoDB + Mongo Express are opt-in.
- Each project runs one small PHP container (php-fpm with nginx or apache).
- Ten projects cost about the RAM of ten php-fpm processes, not ten databases.

## Before you install

- Requirements:
  - macOS or Linux with Docker (Docker Desktop, OrbStack, ...).
  - Windows binaries are built, but not supported at runtime. lodev edits `/etc/hosts` with sudo, and checks ports by connecting to `127.0.0.1`. It names the process holding a port with `lsof`.
- [mkcert](https://github.com/FiloSottile/mkcert) is optional (`brew install mkcert`).
  - Without it, lodev runs mkcert in Docker and tells you how to trust its CA.
- Every install method also installs shell completions.
  - The install script and `make install` install the completion for your `$SHELL` only.
  - The packages install bash, zsh and fish.

## Install script (macOS, Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | sh
```

- It picks the archive for your OS and CPU (x86_64 or arm64).
- It verifies the archive's sha256 against the release's `checksums.txt`.
- It installs `lodev` and its completions under `/usr/local`. It uses sudo only if needed.
- Options:
  - `VERSION=v1.2.3` (default: latest)
  - `PREFIX` (default: `/usr/local`)

To install under your home folder, without sudo:

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | PREFIX="$HOME/.local" sh
```

With a custom `PREFIX`:

- The script prints the lines to add for your `PATH`, and for the completion.
- Shells read completions only from the standard locations, so the lines are needed.

### macOS quarantine

- The script clears `com.apple.quarantine`, so Gatekeeper does not block lodev.
- `curl`, Homebrew and `lodev self-upgrade` never set it. Only a browser download does.
- If you unpacked a browser-downloaded archive yourself, run `xattr -d com.apple.quarantine /path/to/lodev` once.

## Pre-release (nightly)

Every push to `main` rebuilds the `nightly` pre-release.

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | VERSION=nightly sh
```

Or run `lodev self-upgrade --version nightly`.

- A nightly build reports its `git describe` version, e.g. `v0.1.1-3-g238f647`.
- It shows no new-version notice.
- `lodev self-upgrade` keeps it on nightly.
- Back to stable: `lodev self-upgrade --version v1.2.3`.
- Nightly has archives only: no `.deb`, Homebrew or `PKGBUILD`.

## Homebrew (macOS, Linux)

```bash
brew install namnh198/tap/lodev
```

## Debian / Ubuntu (apt)

1. Download `lodev_<version>_amd64.deb` (or `_arm64.deb`) from the [releases](https://github.com/namnh198/lodev/releases).
2. Install it:

```bash
sudo apt install ./lodev_<version>_amd64.deb
```

## Arch Linux

lodev is not on the AUR. Each release ships the `lodev-bin` `PKGBUILD` (and its `.SRCINFO`) as release files.

Build and install it with makepkg. Repeat the steps to upgrade:

```bash
mkdir lodev-bin && cd lodev-bin
curl -fLO https://github.com/namnh198/lodev/releases/latest/download/PKGBUILD
makepkg -si
```

## Nix

```bash
nix profile add github:namnh198/lodev
nix run github:namnh198/lodev -- version
```

- Both commands need flakes.
- Without flakes enabled, add `experimental-features = nix-command flakes` to `~/.config/nix/nix.conf` (or `/etc/nix/nix.conf`).
- Or add `--extra-experimental-features 'nix-command flakes'` after `nix`.
- `flake.nix` builds from source. When `go.mod` changes, its `vendorHash` must follow:
  1. Run `nix build`.
  2. Copy the `got: sha256-…` value from the error into `vendorHash`.
  3. Commit it.

## From source

```bash
make && sudo make install                  # build as yourself, install to /usr/local
make && make install PREFIX=~/.local       # no sudo
```

- `make` writes the completion script for your `$SHELL` only.
- To pick other shells: `make COMPLETION_SHELLS="bash zsh fish"`.

`make` targets:

- `build`: all of linux, darwin and windows, on amd64 and arm64, into `dist/bin/<os>_<arch>/`.
- `completions` (default): `build`, plus the completion script for `$SHELL`.
- `dist`: archives and `checksums.txt`.
- `deb`
- `packaging`
- `packages`: all of `dist`, `deb` and `packaging`.
- `install`, `uninstall`, `test`, `clean`.

The version comes from `git describe`. Override it with `make VERSION=v1.2.3`.

## Completions for paths

Path flags complete paths:

- `db-import -f` offers `.sql`, `.gz` and `.archive` files and folders.
- `db-export -f` offers any file.
- These offer folders only: `create --docroot`, `config --docroot`, `config-global --data-dir`, and `config-global --<service>-mount-path`.
