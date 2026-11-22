---
title: "Cron and daemons"
description: "Scheduled jobs and long-running daemons in the web container."
---

## Cron

`cron: true` runs cron jobs in the web container with [supercronic](https://github.com/aptible/supercronic),
as your user, in `working_dir`, with the container's environment. Without `cron_jobs` the
project type picks the job:

| type | default job |
|---|---|
| magento | `* * * * * php bin/magento cron:run 2>&1 \| grep -v "Ran jobs by schedule" >> var/log/magento.cron.log \|\| true` |
| laravel | `* * * * * php artisan schedule:run >> storage/logs/schedule.log 2>&1` |
| wordpress | `* * * * * wp cron event run --due-now >> wp-content/cron.log 2>&1` |
| php | none: set `cron_jobs` |

Turn it on with `lodev config --cron=true` (edit `cron_jobs` in the file), then `lodev restart`.
`lodev start` writes the jobs to `.lodev/crontab` (generated, git-ignored); `lodev logs` shows
when each job runs.

## Daemons

`web_extra_daemons` runs long-lived dev processes (a Vite dev server, a queue worker) in the web
container under supervisor, as your user, in `directory` (default `working_dir`). A crashed
process is retried with a growing backoff, up to 100 times (`lodev restart` starts it afresh). `lodev start` writes `.lodev/daemons/` (generated, git-ignored); output
appears in `lodev logs`. Daemons are not health-checked, so a crashing one never fails
`lodev start`. Changes apply on `lodev restart`.
