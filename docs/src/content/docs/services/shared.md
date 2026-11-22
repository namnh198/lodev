---
title: "Customizing the shared services"
description: "Tune the shared MySQL, Redis, OpenSearch and other services."
---

Change the shared services' settings, add services, and give them HTTPS routes.

## MySQL and MariaDB settings

- The files are `~/.lodev/services/mysql/lodev.cnf` and `~/.lodev/services/mariadb/lodev.cnf`.
- They are mounted as `/etc/mysql/conf.d/zz-lodev.cnf`.
- `lodev service start` writes each one when it is missing. The defaults:
  - `sql_mode` without `ONLY_FULL_GROUP_BY`
  - `max_allowed_packet=500M`
  - 8-hour timeouts
  - `mysql_native_password` on MySQL 8.0
- After that it leaves your edits alone.
- To apply an edit: `docker restart lodev-mysql` (or `lodev-mariadb`).

## Extra compose files

- Put compose files in `~/.lodev/services/`, named `*.compose.yml` (or `.yaml`).
- `lodev service start` merges them in name order, over the generated `services.compose-base.yaml`.
- The result is `services.compose-full.yaml`. Every `lodev service` command uses it.
- If a file does not merge, `service start` fails. It keeps the last good `services.compose-full.yaml`.
- `service start` writes `example.compose.yml.example` there. It never overwrites it.

Example, `~/.lodev/services/opensearch.compose.yml`:

```yaml
services:
  opensearch:
    mem_limit: 3g
```

## HTTPS routes

To give a service an HTTPS route:

- Set `VIRTUAL_HOST` in its `environment:`. It is comma-separated, and each host ends in your TLD.
- If the service does not listen on port 80, set `VIRTUAL_PORT` too.
- Add `networks: [lodev, default]` to the service.

What `lodev service start` does with the routes:

- It writes the routes of the running services to `~/.lodev/traefik/lodev-services.yaml`.
- It adds the hosts to the certificate and to `/etc/hosts`.
- It fails if a project already uses one of the hosts.
- `traefik.*` labels are ignored, because Traefik no longer reads the Docker socket.

Example, `adminer`:

```yaml
services:
  adminer:
    image: adminer
    networks: [lodev, default]
    environment: {VIRTUAL_HOST: adminer.test, VIRTUAL_PORT: "8080"}
```

## Files in `~/.lodev/traefik/`

- lodev owns `certs.yaml`, `lodev-services.yaml` and `p-*.yaml`.
- Any other `*.yaml` there is yours, e.g. middlewares or path rules.
- Traefik loads your files. lodev never writes or deletes them.
