---
title: "Quick start"
description: "Create and start your first lodev project, then open its site and reach its database."
sidebar:
  order: 2
---

Create a project, start it, open the site and reach the database.

## First run

The first `lodev` command sets lodev up in `~/.lodev`. It skips `help`, `version`, `completion` and `self-upgrade`. It writes:

- `config.yaml` with the defaults
- `projects.json`
- `data/`
- the pinned `docker-compose` and `docker-buildx` in `bin/`, checked against the sha256 lodev ships (not a checksum file fetched next to them)
- the local CA in `rootCA/`, only without mkcert and with Docker running

Commands that talk to Docker stop with `Docker is not running: start Docker Desktop (or the docker service) and try again` when it is not running:

- `start`, `stop`, `restart`, `remove`, `ssh`, `exec`, `logs`, `xdebug`
- `service …`, `poweroff`, `clean`
- `db-export`, `db-import`, `snapshot create|restore`

## 1. Start the shared services

```bash
lodev service start          # Traefik, MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin
```

## 2. Create and start the project

```bash
cd ~/Sites/myshop
lodev create                 # detects Laravel, WordPress, Magento, Next.js or plain PHP
lodev start                  # → https://myshop.test
```

- `lodev start` asks for your sudo password when `/etc/hosts` needs the project's `.test` names. That is the first time, and when a domain changes.
- `lodev create` lists the commands the repo runs, and asks you to approve them. See [commands the repo runs](../features/repo-commands.md).
- From any folder, name the project instead of using the current folder: `lodev start myshop`. This also works for `stop`, `restart`, `remove` and `ssh`.
- Run a tool in the web container with `lodev magento setup:upgrade`, or any command with `lodev exec <cmd>`.

## 3. Open the site and the tools

- `lodev url` prints the project's URL.
- `lodev open` opens it in the browser.
- `lodev describe` lists the project's URLs, services and credentials.

Shared tools:

- Traefik dashboard: <http://localhost:10999>
- phpMyAdmin: <https://phpmyadmin.test>
- Mailpit: <https://mailpit.test>
- RabbitMQ: <https://rabbitmq.test> (guest/guest), once RabbitMQ is enabled
- Mongo Express: <https://mongo.test>, once it is enabled
  - MongoDB login is `root/root`
  - Web containers get `MONGODB_URI`

All ports listen on `127.0.0.1` only.

## 4. Reach the database

- The database is named after the project, with `-` replaced by `_`. `myshop` → `myshop`; `my-shop` → `my_shop`.
- Login: `root`, password `root`. `lodev describe` prints both.

Inside the project container, the connection is already set:

- `DB_HOST=mysql` (`mariadb` for a project that uses MariaDB)
- `DB_PORT=3306`
- `DB_DATABASE=<database>`
- `DB_USERNAME=root`
- `DB_PASSWORD=root`

Load or save a dump. No port has to be published:

```bash
lodev db-import -f dump.sql  # into the project's database
lodev db-export              # writes myshop-<time>.sql
```

Connect a GUI client on your desktop:

1. Publish the MySQL port and apply it:

```bash
lodev config-global --mysql-port=3306
lodev service start
```

2. Connect to `127.0.0.1`, port `3306`, user `root`, password `root`.

`lodev tableplus` (also `sequelace`, `dbeaver`, `mysqlworkbench`, `tablepro`) opens the project's database in that app. It needs the port published.

## Other services in the project container

- `REDIS_HOST=redis`
- `OPENSEARCH_HOST=opensearch`
- `MAIL_HOST=mailpit`, `MAIL_PORT=1025`. PHP `mail()` goes to Mailpit.

If something does not start or answer, see [troubleshooting](troubleshooting.md). For every key of
the project's config, see [project config](../configuration/project.md).
