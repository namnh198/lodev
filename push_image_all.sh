#!/bin/bash

CURRENT_DIR=$(pwd)
VERSION=$1
if [ "${VERSION:-}" = "" ]; then
  export VERSION=$(git describe --tags --always --dirty)
fi

for item in lodev-php lodev-router lodev-webserver; do
  echo "=========== PUSHING $item:${VERSION} ============"
  CONTAINER_DIR="${CURRENT_DIR}/containers/${item}"
  if [ ! -d "$CONTAINER_DIR" ]; then
    echo "Directory $CONTAINER_DIR does not exist. Skipping $item."
    continue
  fi
  cd "$CONTAINER_DIR"
  make push VERSION=${VERSION} || { echo "Failed to push $item:${VERSION}"; exit 1; }
  cd "$CURRENT_DIR"
done
