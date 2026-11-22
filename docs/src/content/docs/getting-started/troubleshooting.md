---
title: "Troubleshooting"
description: "The messages lodev prints when a start, a service or the host setup fails, and what to do about each."
sidebar:
  order: 3
---

Start with `lodev doctor`.

- It checks Docker, the pinned Compose and Buildx, the global config, the ports, the local CA, `/etc/hosts`, the shared services and every project.
- It prints a fix on each row.
- Its exit code is 1 when a check fails.

Then look at the logs and the Docker commands:

- A failing container: `lodev logs -s web`, or `lodev service logs <svc>` for a shared service.
- The Docker commands lodev runs: add `--debug` to any command, e.g. `lodev --debug start`.

## Docker and the shared stack

| Message | Cause | What to do |
|---|---|---|
| `Docker is not running: start Docker Desktop (or the docker service) and try again` | The Docker daemon is not reachable. | Start Docker, then run the command again. |
| `port <n> is in use:` followed by the process holding it | Another program has lodev's HTTP, HTTPS or dashboard port, or a database port you published. lodev checks them when the shared stack is not running. | Stop that program, or change lodev's port with `lodev config-global --http-port`, `--https-port` or `--dashboard-port`, then `lodev service start`. |
| `stop these projects first: <names>` | `lodev service stop` refuses while projects run. | `lodev stop` each project, or `lodev poweroff` to stop everything. |
| `mongo-express needs mongodb: run lodev service disable mongo-express first` | `disable mongodb` while Mongo Express is on. | Disable `mongo-express` first. |
| `lodev-mysql is not running (lodev service enable mysql)` | `db-export` or `db-import` needs the database service, which is off or stopped. | `lodev service enable mysql`, or `lodev service start`. |
| `could not download docker-compose …` (or `docker-buildx`) | The first command could not fetch lodev's pinned Docker tool. lodev retries when it needs it. | Check the network, or use the system plugin with `lodev config-global --use-docker-compose-system=true` (or `--use-docker-buildx-system=true`). |

## Hosts file and certificates

| Message or symptom | Cause | What to do |
|---|---|---|
| `Updating /etc/hosts (sudo)`, then `sudo …` fails | lodev needs sudo to write the project's names, and the password was not given. | Run the command again and enter your password. |
| `doctor`: `/etc/hosts` `missing: <domains>` | A project or shared-service name is not in the file. | `lodev restart <project>` or `lodev service start` writes it. |
| `/etc/hosts has … without … ; fix the file by hand` | lodev's block in the file is half there. | Remove the partial block by hand, then start again. |
| The browser or `curl` says the certificate is not trusted | lodev's own CA (`~/.lodev/rootCA/rootCA.pem`) is not in your system's trust store. | On macOS: `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/.lodev/rootCA/rootCA.pem`. lodev prints the commands for Linux and Windows when it creates the CA (the `mkcert is not installed…` block). Or install mkcert and run `mkcert -install`, then remove lodev's CA and start again. |
| `doctor`: `Local CA` `no ~/.lodev/rootCA/rootCA.pem` | The CA is created on the first start when Docker runs and mkcert is not installed. | `lodev service start`. |

## Projects and containers

| Message or symptom | Cause | What to do |
|---|---|---|
| `not a lodev project (run lodev create)` | The command ran in a folder with no `.lodev/config.yaml` above it. | `cd` into the project, or name a registered project (`lodev start myshop`). In a new folder, run `lodev create`. |
| `project is not running. run 'lodev start'` | `ssh`, `exec` or a shortcut (`php`, `composer`…) ran while the web container is down. | `lodev start`. |
| `<container> is unhealthy: …` | The web container's checks fail: the project mount, the webserver, php-fpm (or the Next.js server for `nextjs`). The line shows each check's result. | Read `lodev logs -s web`. Also see the `docker logs` and `docker inspect` lines the error prints. |
| `<container> exited (code N)` or `keeps restarting` | The container stopped or crashes on start. | `lodev logs -s web`, and fix the cause it prints. |
| `<container> is not healthy after 120s (container_timeout)` | It did not become healthy in time. A first `nextjs` build or a slow `npm install` can take longer. | Wait and check the logs, or raise `container_timeout` in `~/.lodev/config.yaml` (e.g. `3m`). |
| `project <name> needs lodev <constraint> (this is <version>): run lodev self-upgrade` | The project's `lodev_version_constraint` excludes this lodev. | `lodev self-upgrade`, or `--version` for a version in the range. |
| `hook post-start failed: <step>: exit status N` | A hook step failed. The steps after it did not run, and the command was not undone. | Fix the step, then `lodev restart`. Skip hooks once with `--no-hooks`. |
| `hook <event>: <script> is not executable (chmod +x <script>)` | A script hook is not executable. | `chmod +x` the file. |
| `hook <event>: lodev-<name>-web is not running` | A hook ran while the web container was down. | Start the project first. |
| `Run these commands? [Y/n]` answered no, or `not approved: nothing was registered` | `lodev create` did not get approval for the commands the repo runs. | Run `lodev create` again and approve, or review them in [commands the repo runs](../features/repo-commands.md). |
| `run lodev create in a terminal to review them, or pass --yes` | Run without a terminal. | Review the list, then pass `--yes`. |
| `.lodev/commands is not loaded: run lodev create to review the project's commands` | The folder is a cloned project that is not registered yet. | `lodev create` in that folder. |

## Database clients and GUI apps

- A GUI command (`lodev tableplus`, `sequelace`, `dbeaver`, `mysqlworkbench`, `tablepro`) exits 1 with
  the reason when the project has no MySQL or MariaDB, or its port is not published. Publish it with
  `lodev config-global --mysql-port=3306` (or `--mariadb-port=3307`), then `lodev service start`.
- A client connecting to `127.0.0.1:3306` needs that published port. Inside the project container,
  use `DB_HOST` (`mysql` or `mariadb`) and port 3306 instead.
- When a GUI app is already open, macOS can ignore lodev's arguments for `dbeaver` and
  `mysqlworkbench`: quit the app first.
