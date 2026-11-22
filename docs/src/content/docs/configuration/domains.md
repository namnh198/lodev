---
title: "Domains and TLD"
description: "How project domains and the TLD are chosen."
---

Project domains and the shared services' domains all end in one TLD, set in `~/.lodev/config.yaml`.

## The TLD

- Project domains: `<name>.test`.
- Shared services: `mailpit.test`, `phpmyadmin.test`, `rabbitmq.test`, `mongo.test`.
- Default: `test`.
- A fresh install on Linux writes `local` instead.

To switch every project at once:

```bash
lodev config-global --tld=local
```

What the switch does, in order:

1. Rewrites each project's `domains` in `.lodev/config.yaml`, e.g. `shop.test` → `shop.local`. Comments are kept.
2. Rewrites `projects.json`.
3. Replaces lodev's blocks in `/etc/hosts`.
4. Runs `service start`.
5. Restarts the running projects. Stopped projects pick up the new domains on their next start.

Nothing is written if any project cannot move.

## Choosing a TLD

- On macOS, `.local` goes through mDNS. That is slow, and it may skip `/etc/hosts`.
- On macOS, `test` or `localhost` work better.

## After editing `tld` by hand

Run `lodev config-global --tld=<new> --from-tld=<old>`.
