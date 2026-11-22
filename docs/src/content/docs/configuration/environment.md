---
title: "Environment variables"
description: "Environment variables lodev reads and passes to the web container."
---

Which variables the web container gets, in what order, and how to keep secrets out of git.

## Order

The web container gets three layers. A later layer wins:

1. lodev's own variables, e.g. `DB_HOST`, `REDIS_HOST`.
2. `environment:` from `.lodev/config.yaml`.
3. `.lodev/.env`.

So either of the last two can override `DB_HOST`.

## Rules

- `LODEV_*` names are reserved.
- `.lodev/.env` is for secrets. It is local, git-ignored, and mode 0600.
- The generated `.lodev/lodev.compose-*.yml` files carry its values, so they are 0600 too.

```bash
lodev dotenv set --app-key=abc --mage-run-code=default   # APP_KEY=abc, MAGE_RUN_CODE=default
lodev dotenv get --app-key                                # abc
```

## The `.env` file

- It is a plain `KEY=value` file. Comments, quotes and `export` are allowed.
- You can also edit it by hand.
- Changes apply on `lodev restart`.
- Cron jobs, `lodev ssh` and `lodev exec` see them too.
