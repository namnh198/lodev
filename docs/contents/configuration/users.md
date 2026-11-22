---
title: "User and permissions"
description: "Which user the web container runs as and how file permissions work."
---

`lodev ssh` and `lodev exec` run in the web container as your user (`user:`, default your host
username; lowercased, characters outside `a-z 0-9 _ -` become `_`, a leading digit or `-` gets a
`_` prefix, `root`/`www-data` get `_user`, max 32 chars). `lodev start` / `restart` print one warning when
the name had to change. The user has passwordless `sudo`, the hostname is `<name>-web` and the
workdir is `working_dir` (default `/app`, as on Magento Cloud). Projects started by an older lodev ran in
`/var/www/html`: the first start after the change warns, so you can flush caches that hold paths and
update the IDE's path mapping. nginx / apache / php-fpm workers run as that user too, so files PHP
writes are owned by you on the host. `xdebug` toggling still runs as root.

After upgrading, the first `lodev start` rebuilds `lodev/webserver` (the base image no longer bakes
the host UID into `www-data`). Run `lodev restart` in every project that was running before the
upgrade, or `ssh`/`exec` won't find your user.

`-s`/`--service` picks another service of the project (default `web`): `lodev ssh -s redis-commander`,
`lodev exec -s redis-commander ls`. Those use the image's default user and directory, `bash` if the
image has it, else `sh`. For `exec` the flag only counts before the command.

`ssh`, `exec`, the shortcuts (`php`, `composer`, `magento`…) and `xdebug status` need the project
running: when its web container is down they stop with `project is not running. run 'lodev start'`.
