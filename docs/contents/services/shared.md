---
title: "Customizing the shared services"
description: "Tune the shared MySQL, Redis, OpenSearch and other services."
---

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
