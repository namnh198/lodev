#!/bin/sh
# lodev-node-install <install command...>: runs it unless nothing it depends on changed since the last success.
# The hash includes the package manager's record in node_modules, which the install rewrites: so it is taken after.
stamp=node_modules/.lodev-install
digest() {
  { printf '%s\n' "$*"; node --version
    # .npmrc / .yarnrc.yml change the layout; yarn PnP records its install in .pnp.cjs and .yarn/.
    for f in package.json pnpm-lock.yaml yarn.lock package-lock.json pnpm-workspace.yaml .npmrc .yarnrc.yml \
      .pnp.cjs .yarn/install-state.gz node_modules/.modules.yaml node_modules/.package-lock.json node_modules/.yarn-integrity node_modules/.yarn-state.yml; do
      [ -f "$f" ] && md5sum "$f"
    done; } | md5sum | cut -d' ' -f1
}

if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$(digest "$@")" ]; then
  echo "lodev: dependencies unchanged since the last install; skipping $*"
  exit 0
fi
"$@" || exit
mkdir -p node_modules && digest "$@" > "$stamp"
