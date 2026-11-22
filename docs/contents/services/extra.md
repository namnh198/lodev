---
title: "Extra services"
description: "Add your own compose services to a project."
---

Any `.lodev/*.compose.yml` (or `.yaml`) is merged into the project's compose stack
(`.lodev/lodev.compose-full.yml`, generated and ignored by version control):

```yaml
# .lodev/redis-commander.compose.yml
services:
  redis-commander:
    image: rediscommander/redis-commander
    environment: [REDIS_HOSTS=local:redis:6379]
    networks: [lodev, default]   # lodev: reach redis/mysql; default: talk to web
```

Don't name an extra service `mysql`, `redis`, `opensearch`, `mailpit` or `phpmyadmin`: it would
collide with the shared ones on the `lodev` network. On the `lodev` network the web container is
reachable as `lodev-<name>-web` (the bare `web` alias exists on every project's `lodev` network, so
it is ambiguous); `web` works only on the project's `default` network. `lodev create` scaffolds `example.compose.yml.example`.

An extra service gets an HTTPS route by setting `VIRTUAL_HOST` in its `environment:` (one host, or
a comma-separated list, each ending in your TLD) and `VIRTUAL_PORT` (the container port, default 80).
It needs `networks: [lodev, default]`: Traefik reaches it on `lodev`, as `lodev-<project>-<service>-1`
(or its `container_name`).

```yaml
# .lodev/adminer.compose.yml
services:
  adminer:
    image: adminer
    networks: [lodev, default]
    environment:
      VIRTUAL_HOST: adminer.test          # one host, or several: a.test,b.test
      VIRTUAL_PORT: "8080"                # container port; default 80
```

`lodev start` adds these hosts to the project's certificate, `/etc/hosts` and the web container's
`extra_hosts`, and fails if another project or a shared service already uses one. It writes the
project's routes to `~/.lodev/traefik/p-<project>.yaml`; `stop`, `remove` and `poweroff` delete
it. `traefik.*` labels are ignored (Traefik reads route files, not the Docker socket):
use `VIRTUAL_HOST` / `VIRTUAL_PORT` instead.
