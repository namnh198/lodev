---
title: "PhpStorm"
description: "Write PhpStorm interpreter, debug server and path mapping into .idea/."
---

`lodev phpstorm` writes the project's PhpStorm settings into `.idea/`, then opens PhpStorm.

## Steps

1. Start the project with `lodev start`.
2. Close the project in PhpStorm, if it is open.
3. Run `lodev phpstorm` in the project folder.

## What it writes

- `php.xml`:
  - A Docker Compose CLI interpreter named `lodev`.
  - Its configuration file is `.lodev/lodev.compose-full.yml`, service `web`, connecting to the running container.
  - Its PHP executable is `/usr/bin/php<php>`.
- `workspace.xml`:
  - A server named after `php_ide_config`, default the project name, for the first domain.
  - A path mapping: the project folder → `working_dir` (default `/app`).
  - The container sets `PHP_IDE_CONFIG` to that server name.
  - It makes `lodev` the project's interpreter.

Every other setting in `.idea/` is kept.

## Running it again

- Running it again only refreshes lodev's entries.
- Run it again after changing `php:`, `working_dir`, `php_ide_config` or domains.

## Opening PhpStorm

- Once `.idea/` is written, it opens PhpStorm on the project folder.
  - macOS: `open -a PhpStorm`.
  - Elsewhere: `phpstorm`.
- If that fails, it warns, and you open the project yourself.
- PhpStorm may overwrite `workspace.xml` when it saves. Close the project first.
- The command asks while PhpStorm runs. `--yes` skips the question.

## Ports

- PhpStorm listens on ports 9000 and 9003 by default.
- Xdebug (after `lodev xdebug on`) connects to `host.docker.internal:9003`.

## Docker interpreter

A plain **Docker** interpreter on the `lodev/webserver` image gets the newest PHP.

- Set its PHP executable to `/usr/bin/php<version>`, e.g. `/usr/bin/php8.4`.
