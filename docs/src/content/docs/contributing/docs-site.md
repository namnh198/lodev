---
title: "Docs site"
description: "How this site is built and deployed to Cloudflare Workers."
---

How to build, preview and deploy the docs site.

## Stack

- The site is [Starlight](https://starlight.astro.build) (Astro), in `docs/`.
- The pages are the markdown files under `docs/src/content/docs/`.

## Build and preview

```bash
cd docs
pnpm install
pnpm run dev       # live preview
pnpm run build     # builds dist/ and fails on a broken internal link or anchor
```

## Links

- Link between pages with relative file links, e.g. `../configuration/project.md`.
- They work on GitHub, and are rewritten to site URLs at build time.

## Deploy

Cloudflare Workers Builds builds the site on every push to `main` that changes `docs/`.

- It serves `docs/dist/` as a Worker's static assets.
- The site is `lodev.namhoainguyen.com`, set in `docs/wrangler.jsonc`.
- The build fails on a broken internal link or anchor, so a broken docs change is not deployed.
- The Node version comes from `docs/.node-version`.
- The first deploy creates the custom domain and its DNS record, because the zone is on Cloudflare.

### One-time setup

In the Cloudflare dashboard, go to Workers & Pages → Create → Import a repository, and use:

| Setting | Value |
|---|---|
| Repository | `namnh198/lodev` |
| Worker name | `lodev-docs` (must match `name` in `docs/wrangler.jsonc`) |
| Root directory | `docs` |
| Build command | `pnpm install --frozen-lockfile && pnpm run build` |
| Deploy command | `npx wrangler deploy` |
| Build watch paths | Include `docs/*` (relative to the repository root; other pushes are skipped) |
