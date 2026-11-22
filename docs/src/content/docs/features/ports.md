---
title: "Exposed ports"
description: "Publish service and project ports on the host."
---

Makes a server inside the web container (e.g. a Vite dev server) reachable at its own HTTPS host name.

## What it does

- `web_extra_exposed_ports` routes `https://<name>.<first domain>` to a web container port.
  - Example: `https://vite.shop.test`.
- It goes straight to the web container, even with Varnish.
- The host joins the certificate, `/etc/hosts` and the domain checks, like any domain.
- Rules:
  - It needs a webserver.
  - The host cannot equal one of `domains`.
- The web container gets `LODEV_EXPOSED_<NAME>_URL` with the URL.
  - `<NAME>` is the name upper-cased, with `-` turned into `_`.
- Changes apply on `lodev restart`.

## Example: Vite

```yaml
web_extra_daemons:
  - name: vite
    command: npm run dev -- --host 0.0.0.0 --port 5173 --strictPort
web_extra_exposed_ports:
  - name: vite
    container_port: 5173
```

```js
// vite.config.js
const url = process.env.LODEV_EXPOSED_VITE_URL; // https://vite.shop.test
export default { server: {
  host: '0.0.0.0', port: 5173, strictPort: true,
  origin: url, allowedHosts: [new URL(url).hostname],
  cors: { origin: /^https:\/\/([a-z0-9-]+\.)*shop\.test$/ },
  hmr: { host: new URL(url).hostname, protocol: 'wss', clientPort: 443 },
}};
```
