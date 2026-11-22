---
title: "Install"
description: "Install lodev on macOS or Linux: script, Homebrew, apt, Arch, Nix or from source."
sidebar:
  order: 1
---

A lightweight, DDEV-like CLI for local PHP development on macOS with Docker.

Unlike DDEV, the heavy services — MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin (and opt-in MariaDB, RabbitMQ, MongoDB + Mongo Express) — run **once,
shared by all projects**. Each project only runs one small PHP container (php-fpm + nginx or
apache), so ten projects cost about the RAM of ten php-fpm processes, not ten databases.


Requirements: macOS or Linux with Docker (Docker Desktop, OrbStack, …). Windows binaries are built
but not supported at runtime: lodev edits `/etc/hosts` with sudo and checks ports with `lsof`.
[mkcert](https://github.com/FiloSottile/mkcert) is optional (`brew install mkcert`); without it
lodev runs mkcert in Docker and tells you how to trust its CA.

Every method below also installs shell completions: the install script and `make install`
for your `$SHELL` only, the packages for bash, zsh and fish.

## Install script (macOS, Linux)

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

Path flags complete paths: `db-import -f` offers `.sql`, `.gz` and `.archive` files and folders,
`db-export -f` any file, and `create --docroot`, `config --docroot`, `config-global --data-dir` and
`config-global --<service>-mount-path` offer folders only.

On macOS it also clears `com.apple.quarantine`, so Gatekeeper does not block lodev. `curl`,
Homebrew and `lodev self-upgrade` never set it; only a browser download does. If you
unpacked an archive downloaded with a browser yourself, run
`xattr -d com.apple.quarantine /path/to/lodev` once.

## Pre-release (nightly)

Every push to `main` rebuilds the `nightly` pre-release. To try it:

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | VERSION=nightly sh
```

or `lodev self-upgrade --version nightly`. A nightly build reports its `git describe` version
(e.g. `v0.1.1-3-g238f647`), shows no new-version notice, and `lodev self-upgrade` keeps it on nightly.
Go back to stable with `lodev self-upgrade --version v1.2.3`. Nightly has archives only (no `.deb`,
Homebrew or `PKGBUILD`).

## Homebrew (macOS, Linux)

```bash
brew install namnh198/tap/lodev
```

## Debian / Ubuntu (apt)

Download `lodev_<version>_amd64.deb` (or `_arm64.deb`) from the
[releases](https://github.com/namnh198/lodev/releases), then:

```bash
sudo apt install ./lodev_<version>_amd64.deb
```

## Arch Linux

lodev is not on the AUR: each release ships the `lodev-bin` `PKGBUILD` (and its `.SRCINFO`) as
release files. Build and install it with makepkg; repeat to upgrade:

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

Both need flakes. Without `experimental-features = nix-command flakes` in `~/.config/nix/nix.conf`
(or `/etc/nix/nix.conf`), add `--extra-experimental-features 'nix-command flakes'` after `nix`.

`flake.nix` builds from source. Whenever `go.mod` changes, its `vendorHash` must follow: run
`nix build`, copy the `got: sha256-…` value from the error into `vendorHash`, and commit it.

## From source

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
