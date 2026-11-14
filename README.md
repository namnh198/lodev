# lodev

A lightweight, DDEV-like CLI for local PHP development with Docker.

## Getting Started

You need macOS or Linux with Docker (Docker Desktop, OrbStack, …).

```bash
curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | sh
```

You can also install it with `brew install namnh198/tap/lodev`, a `.deb` from the
[releases](https://github.com/namnh198/lodev/releases), `yay -S lodev-bin`, or
`nix profile add github:namnh198/lodev`.

```bash
lodev service start      # shared MySQL, Redis, OpenSearch, Mailpit, phpMyAdmin, Traefik
cd ~/Sites/myshop
lodev create             # detects Laravel / WordPress / Magento / plain PHP
lodev start              # → https://myshop.test
```

`start` asks for your password once to add the `.test` names to `/etc/hosts`. Without
[mkcert](https://github.com/FiloSottile/mkcert), lodev prints the command to trust its local CA.

## Why LODEV?

- **One shared stack.** MySQL, Redis, OpenSearch and Mailpit run once for every project. Each project
  runs a single small PHP container (php-fpm with nginx or apache), so ten projects cost about ten
  php-fpm processes, not ten databases.
- **Ready for real apps.** It supports PHP 7.4–8.5, with Composer, Node.js, pnpm, Yarn, wp-cli and n98-magerun2 in the
  image. It detects the framework and sets up the database, HTTPS domains and mail capture for you.
- **Your config travels with the repo.** `.lodev/config.yaml` holds the project's settings, hooks,
  cron and extra services. Personal toggles such as Xdebug stay in the uncommitted `config.local.yaml`.

## Everyday Commands

| Command | What it does |
|---|---|
| `lodev start` / `stop` / `restart` | Start, stop or restart the current project (`start --all` / `restart --all` for every project) |
| `lodev list` | All projects with status and domains |
| `lodev describe` | URLs, services, ports and credentials of the project |
| `lodev ssh` / `lodev exec <cmd>` | Shell or command in the project's web container |
| `lodev composer` / `npm` / `artisan` / `wp` / `magento` … | Run the tool inside the container |
| `lodev db-import -f dump.sql` / `lodev db-export` | Load or dump the project's database |
| `lodev snapshot create` / `restore` | Save and restore the project's databases |
| `lodev xdebug on` / `off` | Toggle Xdebug (port 9003) |
| `lodev open` | Open the project in the browser |
| `lodev doctor` | Check Docker, ports, certificates and services, with a fix for each problem |
| `lodev self-upgrade` | Upgrade to the latest signed release |

Every command takes `--help`. Until the docs site exists, the full reference lives in
[docs/reference.md](docs/reference.md): configuration, hooks, services and every command.

## License

MIT, see [LICENSE](LICENSE).
