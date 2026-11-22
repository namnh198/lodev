---
title: "Varnish"
description: "Put Varnish in front of the web container."
---

Puts a Varnish cache between Traefik and the web server, per project.

## Turn it on

1. `lodev config --varnish=true` (optionally with `--varnish-version=9.1`).
2. `lodev restart`.

What it does:

- Adds a `lodev-<name>-varnish` container.
- The request path becomes Traefik → `varnish:80` → `web:80`.
- The web container gets `VARNISH_HOST=lodev-<name>-varnish`.

Notes:

- Not available with `webserver: none`.
- `--varnish=false` removes the container on the next start.

## Default VCL

- lodev writes a default VCL to `.lodev/varnish.vcl`.
- Backend: `lodev-<name>-web:80`.
- PURGE and BAN are allowed from the Docker networks.
- Non-GET/HEAD requests and authorized requests are not cached.
- To manage the file yourself, delete its first line.

## Magento

Use Magento's VCL, and point its cache at Varnish:

```bash
bin/magento varnish:vcl:generate --backend-host=lodev-<name>-web --backend-port=80 \
  --access-list=lodev-<name>-web > .lodev/varnish.vcl
bin/magento setup:config:set --http-cache-hosts=lodev-<name>-varnish:80
bin/magento config:set system/full_page_cache/caching_application 2
```
