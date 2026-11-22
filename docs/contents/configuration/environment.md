---
title: "Environment variables"
description: "Environment variables lodev reads and passes to the web container."
---

The web container gets lodev's variables (`DB_HOST`, `REDIS_HOST`, …), then `environment:` from
`.lodev/config.yaml`, then `.lodev/.env`; a later one wins, so either can override `DB_HOST`.
`LODEV_*` names are reserved. `.lodev/.env` is local (git-ignored, mode 0600), for secrets; the
generated `.lodev/lodev.compose-*.yml` that carry its values are 0600 too:

```bash
lodev dotenv set --app-key=abc --mage-run-code=default   # APP_KEY=abc, MAGE_RUN_CODE=default
lodev dotenv get --app-key                                # abc
```

It is a plain `KEY=value` file (comments, quotes and `export` allowed) you can also edit by hand.
Changes apply on `lodev restart`; cron jobs, `lodev ssh` and `lodev exec` see them too.
