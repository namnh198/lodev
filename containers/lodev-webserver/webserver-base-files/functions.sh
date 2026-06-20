#!/usr/bin/env bash

function lodev_custom_init_scripts {
  echo "Loading custom entrypoint config from ${LODEV_WEB_ENTRYPOINT}";
  if ls ${LODEV_WEB_ENTRYPOINT}/*.sh >/dev/null 2>&1; then
    for f in ${LODEV_WEB_ENTRYPOINT}/*.sh; do
      echo "sourcing $f"
      source "$f"
    done
  fi
}
