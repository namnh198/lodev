---
title: "Exposed ports"
description: "Publish service and project ports on the host."
---

`web_extra_exposed_ports` makes a server inside the web container (a Vite dev server, say)
reachable as `https://<name>.<first domain>` (`https://vite.shop.test`), straight to the web
container even with Varnish. The host joins the certificate, `/etc/hosts` and the domain checks
like any domain; it needs a webserver and cannot equal one of `domains`. The web container gets
`LODEV_EXPOSED_<NAME>_URL` (name upper-cased, `-` becomes `_`) with the URL. Changes apply on
`lodev restart`.

Vite with a daemon and an exposed port:

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
