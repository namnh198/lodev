---
title: "Varnish"
description: "Put Varnish in front of the web container."
---

`lodev config --varnish=true` (optional `--varnish-version=9.1`) then `lodev restart` puts a
`lodev-<name>-varnish` container in front of the web server: Traefik → varnish:80 → web:80,
and the web container gets `VARNISH_HOST=lodev-<name>-varnish`. Not available with `webserver: none`.
`--varnish=false` removes the container on the next start.

lodev writes a default VCL to `.lodev/varnish.vcl` (backend `lodev-<name>-web:80`, PURGE/BAN from the Docker
networks, no caching of non-GET/HEAD or authorized requests). Delete its first line to manage it
yourself. For Magento, use Magento's VCL and point its cache at Varnish:

```bash
bin/magento varnish:vcl:generate --backend-host=lodev-<name>-web --backend-port=80 \
  --access-list=lodev-<name>-web > .lodev/varnish.vcl
bin/magento setup:config:set --http-cache-hosts=lodev-<name>-varnish:80
bin/magento config:set system/full_page_cache/caching_application 2
```
