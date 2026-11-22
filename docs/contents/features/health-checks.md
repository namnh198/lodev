---
title: "Health checks"
description: "What lodev doctor and the health checks verify."
---

Every container has a Docker healthcheck, and `lodev start` / `lodev service start` wait for it (up to
`container_timeout`, default 120s). They stop at once when a container exits, keeps restarting or turns
unhealthy, and print the last check and where to look:

```
lodev-shop-web is unhealthy: /app:OK nginx:OK php-fpm:FAILED phpstatus:FAILED
Check:
  lodev logs -s web
  docker logs lodev-shop-web
  docker inspect --format "{{ json .State.Health }}" lodev-shop-web
```

The web container's `lodev-healthcheck` checks the project mount (`.lodev/config.yaml` readable), the
webserver and php-fpm, and PHP through the webserver (`curl http://127.0.0.1/phpstatus`, served on
127.0.0.1 whatever the project's webserver config; other paths on 127.0.0.1 still reach the project).
For [nextjs](nextjs.md) it checks that the Next server accepts connections on `container_port`
(`nextjs:OK`) instead of `phpstatus`, so `lodev start` waits for the install and dev server (or
build) rather than reporting healthy while the site still answers 502. With `webserver: none` it checks the mount and the PHP CLI. Shared services use their own check
(`SHOW DATABASES`, `redis-cli ping`, OpenSearch `_cluster/health`, `mailpit readyz`, …).
