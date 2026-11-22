---
title: "Cron and daemons"
description: "Scheduled jobs and long-running daemons in the web container."
---

Two ways to run background work in the web container: scheduled cron jobs, and long-running daemons.

## Cron

`cron: true` runs cron jobs with [supercronic](https://github.com/aptible/supercronic).

- Runs as your user, in `working_dir`.
- Uses the container's environment.
- Without `cron_jobs`, the project type picks the job:

| type | default job |
|---|---|
| magento | `* * * * * php bin/magento cron:run 2>&1 \| grep -v "Ran jobs by schedule" >> var/log/magento.cron.log \|\| true` |
| laravel | `* * * * * php artisan schedule:run >> storage/logs/schedule.log 2>&1` |
| wordpress | `* * * * * wp cron event run --due-now >> wp-content/cron.log 2>&1` |
| php | none: set `cron_jobs` |

To turn it on:

1. Run `lodev config --cron=true`. Edit `cron_jobs` in the file if you want your own jobs.
2. Run `lodev restart`.

What lodev does:

- `lodev start` writes the jobs to `.lodev/crontab`. It is generated and git-ignored.
- `lodev logs` shows when each job runs.

## Daemons

`web_extra_daemons` runs long-lived dev processes, such as a Vite dev server or a queue worker.

- They run in the web container, under supervisor.
- They run as your user, in `directory` (default `working_dir`).
- A crashed process is retried with a growing backoff, up to 100 times.
- `lodev restart` starts it afresh.
- `lodev start` writes `.lodev/daemons/`. It is generated and git-ignored.
- Output appears in `lodev logs`.
- Daemons are not health-checked. A crashing one never fails `lodev start`.
- Changes apply on `lodev restart`.
