---
title: "User and permissions"
description: "Which user the web container runs as and how file permissions work."
---

Which user runs inside the web container, and who owns the files PHP writes.

## The container user

- `lodev ssh` and `lodev exec` run as the `user:` key. Its default is your host username.
- The name is sanitized:
  - lowercased
  - characters outside `a-z 0-9 _ -` become `_`
  - a leading digit or `-` gets a `_` prefix
  - `root` and `www-data` become `_user`
  - at most 32 characters
- `lodev start` and `restart` print one warning when the name had to change.
- The user has passwordless `sudo`.
- The hostname is `<name>-web`.
- The workdir is `working_dir`: default `/app`, as on Magento Cloud.
- nginx, apache and php-fpm workers run as that user too. Files PHP writes are owned by you on the host.
- `xdebug` toggling still runs as root.

## Moving from an older lodev

- Projects started by an older lodev ran in `/var/www/html`.
- The first start after the change warns about it. Then you can flush the caches that hold paths, and update the IDE's path mapping.
- After upgrading, the first `lodev start` rebuilds `lodev/webserver`. The base image no longer bakes the host UID into `www-data`.
- Run `lodev restart` in every project that was running before the upgrade. Otherwise `ssh` and `exec` won't find your user.

## Other services

- `-s` / `--service` picks another service of the project. The default is `web`.
  - `lodev ssh -s redis-commander`
  - `lodev exec -s redis-commander ls`
- Those use the image's default user and directory, and `bash` if the image has it, else `sh`.
- For `exec`, the flag only counts before the command.

## Running project required

`ssh`, `exec`, the shortcuts (`php`, `composer`, `magento`...) and `xdebug status` need the project running.

- When the web container is down, they stop with `project is not running. run 'lodev start'`.
