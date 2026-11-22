---
title: "Quick start"
description: "Create and start your first lodev project."
sidebar:
  order: 2
---

The first `lodev` command (any but `help`, `version`, `completion`, `self-upgrade`) sets lodev up in
`~/.lodev`: `config.yaml` with the defaults, `projects.json`, `data/`, the pinned `docker-compose` and
`docker-buildx` in `bin/` (checked against the sha256 lodev ships, not a checksum file fetched
next to them), and, without mkcert and with Docker running, the local CA in `rootCA/`.
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
