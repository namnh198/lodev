---
title: "Domains and TLD"
description: "How project domains and the TLD are chosen."
---

Project domains (`<name>.test`) and the shared services' (`mailpit.test`, `phpmyadmin.test`,
`rabbitmq.test`, `mongo.test`) end in `tld` from `~/.lodev/config.yaml`: `test` by default, `local`
for a fresh install on Linux. Switch every project at once:

```bash
lodev config-global --tld=local
```

It rewrites each project's `domains` in `.lodev/config.yaml` (shop.test → shop.local, comments
kept) and `projects.json`, replaces lodev's blocks in `/etc/hosts`, then runs `service start` and
restarts the projects that are running; stopped ones pick the new domains up on their next start.
Nothing is written if any project cannot move. On macOS `.local` goes through mDNS (slow, may skip
`/etc/hosts`), so `test` or `localhost` work better there. After editing `tld` in the file by hand,
run `lodev config-global --tld=<new> --from-tld=<old>`.
