---
title: "PhpStorm"
description: "Write PhpStorm interpreter, debug server and path mapping into .idea/."
---

After `lodev start`, run `lodev phpstorm` in the project folder. It writes into `.idea/`:

- `php.xml`: a Docker Compose CLI interpreter named `lodev` (configuration file
  `.lodev/lodev.compose-full.yml`, service `web`, connect to the running container), PHP
  executable `/usr/bin/php<php>`.
- `workspace.xml`: a server named after `php_ide_config` (default the project name) for the first
  domain, mapping the project folder → `working_dir` (default `/app`); the container sets
  `PHP_IDE_CONFIG` to it. It also makes `lodev` the project's interpreter.

Every other setting in `.idea/` is kept, and running it again only refreshes lodev's entries: do so
after changing `php:`, `working_dir`, `php_ide_config` or domains. Once `.idea/` is written it
opens PhpStorm on the project folder (`open -a PhpStorm` on macOS, `phpstorm` elsewhere); when
that fails it warns and you open the project yourself. Close the project in PhpStorm
first: it may overwrite `workspace.xml` when it saves (the command asks while PhpStorm runs;
`--yes` skips the question). PhpStorm listens on ports 9000 and 9003 by default, and Xdebug
(`lodev xdebug on`) connects on `host.docker.internal:9003`.

A plain **Docker** interpreter on the `lodev/webserver` image gets the newest PHP: set its PHP
executable to `/usr/bin/php<version>` (e.g. `/usr/bin/php8.4`).
