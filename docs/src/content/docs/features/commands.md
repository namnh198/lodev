---
title: "Commands"
description: "Every lodev command and its flags."
---

Every `lodev` command, with its aliases and flags.

| Command | What it does |
|---|---|
| `lodev service start\|stop\|status\|logs [svc]` | Manage the shared stack (aliases `services`, `svc`; `up`/`down` for start/stop). `stop` refuses while a project runs (stop it first). `stop --volumes` deletes all shared data (asks first; `--yes` skips) |
| `lodev service enable\|disable <svc>...` | Turn shared services on/off, several at once (data is kept). `disable` removes their containers; `disable mongodb` needs `mongo-express` disabled first |
| `lodev create [--name --type --php --webserver --docroot --no-ask --yes]` | Create `.lodev/config.yaml` (alias `init`). On a terminal an interactive form asks name/type/PHP/docroot/webserver (detected values preselected; no PHP or docroot for [nextjs](nextjs.md)); `--no-ask` skips it. Defaults: PHP `8.4`, webserver `nginx`, the type's docroot; `--docroot .` = project root. It also registers the project in `~/.lodev/projects.json`; in a folder that already has a `.lodev/config.yaml` (a cloned project, or after `lodev remove`) it warns and registers that config as it is; either way it first asks about the [commands the repo runs](repo-commands.md) (`--yes` approves them) |
| `lodev dotenv set --key=value [...]` / `lodev dotenv get --key` | Set or print variables in `.lodev/.env` (`--app-key` is `APP_KEY`); apply with `lodev restart` |
| `lodev config [flags]` | Show or change `.lodev/config.yaml`; validated before saving, keeps comments; apply with `lodev restart`. Flags: `--type`, `--php-version`, `--webserver`, `--docroot`, `--container-port`, `--nodejs-version` (`""` removes the project's Node.js, so the global `node_version` applies), `--user`, `--varnish`, `--varnish-version`, `--cron`, `--performance-mode`. The lists `--domains`, `--services`, `--php-extensions`, `--packages` and `--volume-dirs` take `--X=a,b` to replace, or `--add-X` / `--remove-X`. The local keys `--enable-xdebug` and `--opcache-validate-timestamps` go to `.lodev/config.local.yaml`. Keys without a flag are edited in the file: see [project config](../configuration/project.md) |
| `lodev config-global [flags]` | Show or change `~/.lodev/config.yaml`; apply with `lodev service start`. Flags: `--http-port`, `--https-port`, `--dashboard-port`, `--mysql-port`, `--mariadb-port`, `--mongodb-port` (publish a database on the host; `""` unpublishes), `--mysql-version` (likewise `--mariadb-version`, `--redis-version`, `--opensearch-version`, `--rabbitmq-version`, `--mongodb-version`), `--container-timeout`, `--node-version`, `--opensearch-memory`, `--update-check`, `--auto-create-db`, `--use-docker-compose-system`, `--use-docker-buildx-system`, `--performance-mode`, `--data-dir`, `--<service>-mount-path` (mysql, mariadb, redis, opensearch, rabbitmq, mongodb), `--enable-services`, `--disable-services`, `--tld` (with `--from-tld` for a hand-edited TLD). See [global config](../configuration/global.md) |
| `lodev start [--rebuild \| --rebuild-base \| --all] [--no-hooks]` | Start the project (alias `up`); `--rebuild` rebuilds the project image without cache, `--rebuild-base` the PHP image(s) too; `--all` starts every known project from any folder (it cannot be combined with the rebuild flags). `--no-hooks` skips `post-start`. `start` and `restart` need a project made by `lodev create` (a `.lodev/config.yaml` registered in `~/.lodev/projects.json`) |
| `lodev stop [--no-hooks]` / `lodev restart [--all] [--no-hooks]` | Stop / restart the project container; `--no-hooks` skips `pre-stop` (and `post-start` on restart); `restart --all` restarts every known project |
| `lodev remove [--db] [--volumes] [--yes]` | Remove container, hosts entries, cert (alias `down`); `--db` drops the database; `--volumes` deletes the performance_mode volumes (they are kept and listed otherwise) |
| `lodev list` | All projects with status, type, domains and path (`~` for your home) as a table (alias `ps`) |
| `lodev poweroff` | Remove every project's containers and networks and take the shared services down (data, project images, hosts entries and certificates are kept) |
| `lodev db-export [db \| -n db] [-s svc] [-f file] [-c gz] [-e 'options']` | Dump a database of the shared `mysql`, `mariadb` or `mongodb` (default: the project's database and service) to `<db>-<time>.sql[.gz]` (`.archive` for MongoDB), or `-f -` for stdout. `-e` adds options to mysqldump / mariadb-dump / mongodump, split like a shell (quotes work, repeatable), e.g. `lodev db-export -c gz -f dump.sql.gz -e '--skip-add-locks'` |
| `lodev db-import [db \| -n db] -f file [-s svc] [-e 'options']` | Load a dump into a database (created if missing; plain or gzip, detected); replaces its tables/collections; `-f -` reads stdin. Into a Magento project's own database it also rewrites the settings listed below (`--no-magento` skips it). `--no-hooks` skips `post-db-import`. `-e` adds options to mysql / mariadb / mongorestore, like db-export's. MongoDB dumps can land in another database name |
| `lodev snapshot create\|list\|restore\|delete [name] [-p project]` | Save the project's databases (its MySQL/MariaDB one, plus MongoDB) to `~/.lodev/snapshots/<project>/<name>/` (readable by you only). `create` names it with the time when no name is given; `--force` replaces a snapshot of the same name. `list` shows them. `restore` loads one back (default: the newest; asks first, `--yes` skips; checks every file of the snapshot before dropping anything, so a damaged one changes nothing; runs `post-snapshot-restore`, `--no-hooks` skips it). `delete` removes one |
| `lodev logs [-s service] [-j]` | Follow a service's logs (default web; `-s mysql` for a shared service); `-j` pretty-prints JSON lines with jq (web only) |
| `lodev ssh [name] [-s service]` / `lodev exec [-s service] <cmd>` | Shell / command in the web container, or in another service of the project. The project must be running |
| `lodev php\|composer\|npm\|node\|pnpm\|yarn\|grunt\|artisan\|wp\|magento\|n98 ...` | Shortcuts, shipped [custom commands](custom-commands.md) (`n98` runs `n98-magerun2`); `artisan` exists only in laravel projects, `wp` in wordpress ones, `magento` and `n98` in magento ones |
| `lodev <name> [args...]` | A [custom command](custom-commands.md) from `~/.lodev/commands/` or `.lodev/commands/` |
| `lodev vscode` | Open VS Code attached to the web container (needs `code` on PATH and the Dev Containers extension; starts the project if stopped). A shipped host [custom command](custom-commands.md) |
| `lodev tableplus\|sequelace\|dbeaver\|mysqlworkbench\|tablepro` | Open the project's MySQL/MariaDB database in that GUI app. Shipped host [custom commands](custom-commands.md); they need `ports.mysql` or `ports.mariadb` published (`lodev config-global --mysql-port=3306`, then `lodev service start`) |
| `lodev xdebug on\|off\|toggle\|status` | Turn Xdebug on/off, or flip it with `toggle` (port 9003); saved as `enable_xdebug` in `.lodev/config.local.yaml` (never the committed `config.yaml`), so `lodev start` / `restart` keep it (a stopped project just gets the setting) |
| `lodev phpstorm [--yes]` | Write the PhpStorm interpreter, debug server and path mapping into `.idea/` ([PhpStorm](phpstorm.md)), keeping your other settings, then open PhpStorm |
| `lodev describe [project]` | Project URLs, its services with status, in-network/host ports, images and credentials, plus phpMyAdmin/Mailpit (aliases `status`, `st`, `desc`) |
| `lodev open [project\|phpmyadmin\|mailpit\|rabbitmq\|mongo-express\|traefik] [--print]` | Open the project here (or another one, or a shared service page) in the browser; `--print` only prints the URL. A shipped host [custom command](custom-commands.md) (`open` on macOS, `xdg-open` on Linux) |
| `lodev url [project\|phpmyadmin\|mailpit\|rabbitmq\|mongo-express\|traefik]` | Print the URL of the project here (or another one, or a shared service page) |
| `lodev clean [--dry-run] [--yes] [--keep-cache]` | Remove lodev images no container uses (older versions, `lodev/php`, removed projects, and `ghcr.io/namnh198/lodev-webserver` images: older ones on a stable release, all of them on nightly, which builds its own) and Docker build cache older than 7 days; lists them and asks first; never touches volumes |
| `lodev doctor` | Check Docker, the pinned compose/buildx, the global config, ports, the local CA, `/etc/hosts`, the shared services, the projects, userns-remap and Docker disk use; each row says how to fix it; exits 1 on a problem. See [troubleshooting](../getting-started/troubleshooting.md) |
| `lodev self-upgrade [--version vX.Y.Z]` | Replace lodev (and the completions installed next to it) with the latest release, or the given one; verifies the release's signature and sha256 (unsigned releases are refused) and asks for your sudo password when the folder needs root. Homebrew, apt, pacman (the release's `PKGBUILD`) and Nix installs are upgraded their own way instead |
| `lodev version` | lodev, Docker, Docker Compose and Docker Buildx versions, OS and architecture; `lodev -v` prints only `lodev version <v>` |
| `lodev --debug <cmd>` | Also print every docker command lodev runs (or set `LODEV_DEBUG=1`); set `NO_COLOR=1` to turn colors off. `LODEV_HOME` moves lodev's state folder (see [global config](../configuration/global.md)) |

### Magento settings after `db-import`

When the import goes into a Magento project's own database, lodev:

- Sets the base URLs on every scope to `https://<domain>/`.
- Clears the CDN static and media URLs, and the cookie domain.
- Sends mail to Mailpit (`system/smtp/*`, on every scope).
- Points the search engine set in `catalog/search/engine` at `opensearch:9200`.
  - The engine is OpenSearch or Elasticsearch; default `opensearch`.
  - No auth is used. The index prefix is the project name.
- Flushes the cache.

`--no-magento` keeps the dump's own settings.

**Known limit:** a running project learns about domains of projects started after it on its
next `lodev start` / `lodev restart`.
