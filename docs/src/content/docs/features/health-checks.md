---
title: "Health checks"
description: "What lodev doctor and the health checks verify."
---

Every container has a Docker healthcheck. `lodev start` and `lodev service start` wait for it.

## Waiting

- The wait lasts up to `container_timeout` (default 120s).
- lodev gives up at once when a container:
  - exits
  - keeps restarting
  - turns unhealthy
- It then prints the last check, and where to look:

```
lodev-shop-web is unhealthy: /app:OK nginx:OK php-fpm:FAILED phpstatus:FAILED
Check:
  lodev logs -s web
  docker logs lodev-shop-web
  docker inspect --format "{{ json .State.Health }}" lodev-shop-web
```

## What the web container checks

The web container's `lodev-healthcheck` checks:

- the project mount (`.lodev/config.yaml` readable)
- the webserver and php-fpm
- PHP through the webserver: `curl http://127.0.0.1/phpstatus`
  - served on 127.0.0.1, whatever the project's webserver config is
  - other paths on 127.0.0.1 still reach the project

Per type and webserver:

- [nextjs](nextjs.md): checks that the Next server accepts connections on `container_port` (`nextjs:OK`), instead of `phpstatus`.
  - So `lodev start` waits for the install and the dev server (or the build).
  - Otherwise it would report healthy while the site still answers 502.
- `webserver: none`: checks the mount and the PHP CLI.

## Shared services

Each shared service uses its own check, for example:

- `SHOW DATABASES` for MySQL
- `redis-cli ping` for Redis
- `_cluster/health` for OpenSearch
- `readyz` for Mailpit
