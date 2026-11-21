---
paths:
  - "cmd/trust.go"
  - "cmd/hooks.go"
  - "cmd/project.go"
  - "cmd/prompt.go"
  - "internal/config/**"
---

# Commands from the project's repo

- A cloned repo can make lodev run code: hooks (and the scripts and `.d` files they name), cron,
  `web_extra_daemons`, a nextjs project's `dev`, `build` and `start`/`preview` scripts (or a taken-over `.lodev/daemons/nextjs.sh`), `.lodev/*.compose.yml`, `.lodev/web-images/`, `.lodev/homeadditional/`,
  `environment:`, `.lodev/.env` (names only: it holds secrets), and apache config. `trustItems`
  in `cmd/trust.go` lists every one of them.
- A new config key or `.lodev/` folder that lodev executes, mounts or copies into the container
  gets an entry in `trustItems` and a `TestTrustItems*` case in the same change.
- `confirmRepoCommands` asks once, at `lodev create`; nothing is stored, and `start`, `restart`
  and hooks never ask (the accepted risk is in `docs/reference.md`, "Commands from the project's repo").
- `*.example` files are scaffolding: never listed, copied or executed.
- `scaffold` runs on every `start`: it must be idempotent, keep user edits, and leave a
  symlinked `.lodev/<dir>` alone.
