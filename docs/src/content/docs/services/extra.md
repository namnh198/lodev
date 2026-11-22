---
title: "Extra services"
description: "Add your own compose services to a project."
---

Add your own containers to one project, next to its web container.

## Adding a service

- Any `.lodev/*.compose.yml` (or `.yaml`) is merged into the project's compose stack.
- The merged file is `.lodev/lodev.compose-full.yml`. It is generated and git-ignored.

```yaml
# .lodev/redis-commander.compose.yml
services:
  redis-commander:
    image: rediscommander/redis-commander
    environment: [REDIS_HOSTS=local:redis:6379]
    networks: [lodev, default]   # lodev: reach redis/mysql; default: talk to web
```

Rules:

- Do not name an extra service `mysql`, `redis`, `opensearch`, `mailpit` or `phpmyadmin`. It would collide with the shared ones on the `lodev` network.
- `lodev create` scaffolds `example.compose.yml.example`.

## Reaching the web container

- On the `lodev` network, the web container is `lodev-<name>-web`.
- The bare `web` alias exists on every project's `lodev` network, so it is ambiguous.
- `web` works only on the project's `default` network.

## HTTPS route

An extra service gets an HTTPS route with `VIRTUAL_HOST` and `VIRTUAL_PORT`, set in its `environment:`:

- `VIRTUAL_HOST`: one host, or a comma-separated list. Each ends in your TLD.
- `VIRTUAL_PORT`: the container port. Default 80.
- `networks: [lodev, default]` is required. Traefik reaches the service on `lodev`, as `lodev-<project>-<service>-1` (or its `container_name`).

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

What `lodev start` does with those hosts:

- Adds them to the project's certificate, `/etc/hosts` and the web container's `extra_hosts`.
- Fails if another project or a shared service already uses one.
- Writes the project's routes to `~/.lodev/traefik/p-<project>.yaml`.
- `stop`, `remove` and `poweroff` delete that file.
- `traefik.*` labels are ignored, because Traefik reads route files, not the Docker socket. Use `VIRTUAL_HOST` and `VIRTUAL_PORT` instead.
