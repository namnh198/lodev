---
title: "Next.js"
description: "Run a Next.js front end next to the PHP back end."
---

Runs a Next.js app behind its own domain, next to the PHP back end.

## Setup

- `type: nextjs` serves the app at `https://<name>.test`, with no `:3000` in the URL.
- `lodev create` picks it when the project root has `next.config.ts`, `next.config.js` or `next.config.mjs`.
  - The check runs after the Magento, Laravel and WordPress checks.
- It needs `webserver: nginx`.
- `nextjs` is not available as a `web_extra_daemons` name.
- `lodev create` gives a nextjs project no shared services. It asks no PHP version or docroot.

## Package manager

The package manager follows the lockfile, checked on every start. The first file present wins:

- `pnpm-lock.yaml` → pnpm
- `yarn.lock` → yarn
- `package-lock.json` → npm
- none → pnpm

With `performance_mode: bind`, `node_modules` (when there is a `package.json`) and `.next` live in Docker volumes. They start empty, so the container's Linux packages never land in your host folders.

## The `nextjs` daemon

On start, lodev adds a daemon named `nextjs` (see [Daemons](cron-daemons.md#daemons); its output is in `lodev logs`). It installs the dependencies, then runs your `dev` script on `container_port` (default 3000).

| Lockfile | Install | Then |
|---|---|---|
| `pnpm-lock.yaml` | `pnpm install --frozen-lockfile` | `PORT=3000 exec pnpm run dev` |
| `yarn.lock` | `yarn install` | `PORT=3000 exec yarn run dev` |
| `package-lock.json` | `npm ci` | `PORT=3000 exec npm run dev` |
| none | `pnpm install` | `PORT=3000 exec pnpm run dev` |

### When the install is skipped

The install is skipped when none of these changed since the last successful install:

- the lockfile, `package.json`, `pnpm-workspace.yaml`, `.npmrc` or `.yarnrc.yml`
- the Node version or the install command
- the package manager's record of its last install: `node_modules/.modules.yaml`, `.package-lock.json`, `.yarn-integrity`, `.yarn-state.yml`, yarn PnP's `.pnp.cjs`, or `.yarn/install-state.gz`

When it is skipped, the log says `lodev: dependencies unchanged since the last install; skipping …`.

The install state lives in `node_modules/.lodev-install`. So these run the install again:

- a new or emptied `node_modules`
- an install run on the host

To force an install, for example after deleting part of `node_modules`:

1. Delete `node_modules/.lodev-install`.
2. Run `lodev restart`.

### Production mode

With `NODE_ENV=production` in the container, the daemon runs a build and a server instead:

- `<pm> run build && PORT=3000 exec <pm> run start`
- It uses `preview` in place of `start` when `package.json` has no `start` script.
- The install always runs with `NODE_ENV=development`, so the devDependencies that `next build` needs (TypeScript, `@types/*`) are there.
- Set `NODE_ENV=production` through `environment:` or `.lodev/.env`, then run `lodev restart`.
- A build can outlast the default `container_timeout` (120s). Raise it in `~/.lodev/config.yaml` when `lodev start` reports the container as not healthy in time.

## Port and proxy

- `PORT` is `container_port`. For example: `lodev config --container-port=4000`, then `lodev restart`.
- nginx keeps listening on 80 and proxies to that port.
- `container_port` must not be 80, and no `web_extra_exposed_ports` entry may use it.
- Other project types accept only 80.
- The generated `.lodev/nginx-webserver.conf` proxies every request to the port, websockets included (HMR).
  - Delete its marker line to take it over.

## Taking over the script

- `.lodev/daemons/nextjs.sh` starts with `# lodev-generated: delete this line to manage this file yourself`.
- Delete that line to run your own command, for example `exec bun dev`. It must serve `container_port`.
- On start, lodev then prints `.lodev/daemons/nextjs.sh is user-managed; not regenerating`.
- lodev never rewrites or deletes the file.
- lodev stops git-ignoring it, so you can commit it.

## Environment

- The web container gets `NODE_EXTRA_CA_CERTS=/etc/lodev/rootCA.pem`.
- With it, server-side `fetch` trusts other lodev projects' https domains, e.g. `https://magento2.test/graphql`.
- `environment:` or `.lodev/.env` can override it.

## Repo commands

The [commands the repo runs](repo-commands.md) list the nextjs entry as:

- `nextjs: <install> && <pm> run dev, or with NODE_ENV=production <pm> run build && <pm> run start`
- It shows the `dev`, `build` and `start` scripts from `package.json`.
- A missing script shows as `none`.
- `preview` replaces `start` when only that script exists.

## Start-time warnings

`lodev start` warns when:

- `performance_mode` is `none`, the host is not Linux, and the project has `node_modules` or `.next`. The container installs Linux packages into them.
- The root `next.config.*` does not mention the first domain. Next.js blocks its dev assets on other hosts. Add `allowedDevOrigins: ["<name>.test"]` to it.

lodev never edits `next.config.*` or `package.json`.
