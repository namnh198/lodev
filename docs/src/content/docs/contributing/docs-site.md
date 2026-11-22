---
title: "Docs site"
description: "How this site is built and deployed to Cloudflare Workers."
---

The site is [Starlight](https://starlight.astro.build) (Astro) in `docs/`; the pages are the
markdown files under `docs/src/content/docs/`.

```bash
cd docs
pnpm install
pnpm run dev       # live preview
pnpm run build     # builds dist/ and fails on a broken internal link or anchor
```

Link between pages with relative file links (`../configuration/project.md#local-overrides`):
they work on GitHub and are rewritten to site URLs at build time.

## Deploy

Cloudflare Workers Builds builds the site on every push to `main` that changes `docs/`, and serves `docs/dist/` as a
Worker's static assets, on `lodev.namhoainguyen.com` (`docs/wrangler.jsonc`). One-time setup in the
Cloudflare dashboard, under Workers & Pages → Create → Import a repository:

| Setting | Value |
|---|---|
| Repository | `namnh198/lodev` |
| Worker name | `lodev-docs` (must match `name` in `docs/wrangler.jsonc`) |
| Root directory | `docs` |
| Build command | `pnpm install --frozen-lockfile && pnpm run build` |
| Deploy command | `npx wrangler deploy` |
| Build watch paths | Include `docs/*` (relative to the repository root; other pushes are skipped) |

The first deploy creates the custom domain and its DNS record, because the zone is on Cloudflare.
The Node version comes from `docs/.node-version`. The GitHub workflow `docs.yml` only checks that the
site builds; it deploys nothing.
