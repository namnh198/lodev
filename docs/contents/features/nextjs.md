---
title: "Next.js"
description: "Run a Next.js front end next to the PHP back end."
---

`type: nextjs` runs a Next.js app behind its domain (`https://<name>.test`, no `:3000`). `lodev create`
picks it when the project root has `next.config.ts`, `next.config.js` or `next.config.mjs` (after the
Magento, Laravel and WordPress checks). It needs `webserver: nginx`, and `nextjs` is not available as a
`web_extra_daemons` name there. The package manager follows the lockfile, checked on every start:
`pnpm-lock.yaml` → pnpm, `yarn.lock` → yarn, `package-lock.json` → npm, none → pnpm (the first one
present wins). With `performance_mode: bind`, `node_modules` (when there is a `package.json`) and
`.next` live in Docker volumes, empty at first, so the container's Linux packages never land in the
host's folders.

On start lodev adds a daemon named `nextjs` (see [Daemons](cron-daemons.md#daemons); output in `lodev logs`) that
installs, then runs your `dev` script on `container_port` (default 3000):

| Lockfile | Install | Then |
|---|---|---|
| `pnpm-lock.yaml` | `pnpm install --frozen-lockfile` | `PORT=3000 exec pnpm run dev` |
| `yarn.lock` | `yarn install` | `PORT=3000 exec yarn run dev` |
| `package-lock.json` | `npm ci` | `PORT=3000 exec npm run dev` |
| none | `pnpm install` | `PORT=3000 exec pnpm run dev` |

The install is skipped when the lockfile, `package.json`, `pnpm-workspace.yaml`, `.npmrc`,
`.yarnrc.yml`, the Node version, the install command and the package manager's record of its last
install (`node_modules/.modules.yaml`, `.package-lock.json`, `.yarn-integrity`, `.yarn-state.yml`;
yarn PnP's `.pnp.cjs` and `.yarn/install-state.gz`) are unchanged since the last successful
install; the log then says `lodev: dependencies unchanged since the last install; skipping …`. That
state lives in `node_modules/.lodev-install`, so a new or emptied `node_modules` installs again, and
so does an install run on the host. To force one (e.g. after deleting part of `node_modules`),
delete `node_modules/.lodev-install` and `lodev restart`.

With `NODE_ENV=production` in the container (`environment:` or `.lodev/.env`, then `lodev restart`)
it runs `<pm> run build && PORT=3000 exec <pm> run start` instead: `start`, or `preview` when
`package.json` has no `start` script. The install always runs with `NODE_ENV=development`, so
devDependencies `next build` needs (TypeScript, `@types/*`) are there. A build can outlast the
default `container_timeout` (120s); raise it in `~/.lodev/config.yaml` when `lodev start` reports
the container is not healthy in time.

`PORT` is `container_port` (e.g. `lodev config --container-port=4000`, then `lodev restart`); nginx
keeps listening on 80 and proxies to that port. It must not be 80, no `web_extra_exposed_ports`
entry may use it, and other project types accept only 80.

The generated `.lodev/nginx-webserver.conf` proxies every request to it, websockets included (HMR);
delete its marker line to take it over. The daemon's script, `.lodev/daemons/nextjs.sh`, works the
same way: it starts with `# lodev-generated: delete this line to manage this file yourself`; delete
that line to run your own command (e.g. `exec bun dev`; it must serve `container_port`). lodev then prints
`.lodev/daemons/nextjs.sh is user-managed; not regenerating` on start, never rewrites or deletes it,
and stops git-ignoring it so you can commit it. The web container gets
`NODE_EXTRA_CA_CERTS=/etc/lodev/rootCA.pem`, so server-side `fetch` trusts other lodev projects'
https domains (`https://magento2.test/graphql`); `environment:` or `.lodev/.env` can override it.

`lodev create` gives a nextjs project no shared services and asks no PHP version or docroot. The
[commands the repo runs](repo-commands.md) list it as `nextjs: <install> && <pm> run
dev, or with NODE_ENV=production <pm> run build && <pm> run start (package.json dev: "<script>",
build: "<script>", start: "<script>")` (`none` for a missing script; `preview` in place of `start`
when only that exists). `lodev start` warns when `performance_mode` is `none`, the host
is not Linux and it has `node_modules` or `.next` (the container installs Linux packages into them), and when the
root `next.config.*` does not mention the first domain: Next.js blocks its dev assets on other
hosts, so add `allowedDevOrigins: ["<name>.test"]` to it. lodev never edits `next.config.*` or
`package.json`.
