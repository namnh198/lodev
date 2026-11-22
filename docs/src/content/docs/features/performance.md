---
title: "Performance mode"
description: "Trade file-sync safety for speed on bind mounts."
---

Keeps some project folders in Docker volumes, which is faster than the bind mount, on macOS.

## Setting the mode

- `performance_mode: bind` is the default on macOS, where bind mounts are slow. Linux defaults to `none`.
- Set it per project: `lodev config --performance-mode=bind|none`.
- Or for all projects: `lodev config-global --performance-mode=...`.
- An empty value (`""`) goes back to the next level: project, then global, then the OS default.

## Folders kept in volumes

| Type | Folders in volumes |
|---|---|
| magento | `var`, `generated`, `node_modules`* |
| laravel, wordpress, php | `node_modules`* |

\* only when `package.json` exists at the project root.

## Your own folders

- Laravel's `storage` stays on the bind mount, because it holds real data. Add it to `volume_dirs` to put it in a volume.
- `volume_dirs` adds your own folders, e.g. `lodev config --add-volume-dirs=pub/media`.
- Rules for `volume_dirs`:
  - relative paths made of A-Z, a-z, 0-9, `.`, `_` and `-` segments
  - not `.lodev`
  - no folder inside another one in volumes
  - no two that map to the same volume name, `lodev-<name>-<folder with / as ->`

## What to expect

- `var`, `generated` and `node_modules` start empty in their volume.
- Every `volume_dirs` folder is copied from the host once, when its volume is created.
- The host no longer sees what the container has in these folders.
  - Read logs with `lodev logs` or `lodev exec tail`.
  - Host-side `npm`, or IDE indexing of `generated/`, needs `performance_mode: none`.
- `lodev start` creates the volumes before the containers. The volumes are labelled `lodev.project=<name>`.
- `lodev start` prints what each folder got.
- A folder that is a symlink on the host is skipped, with a warning.
- Files the container writes into a seeded folder (e.g. uploads in `pub/media`) live only in its volume.
- `lodev describe` shows the effective mode and folders.

## Removing volumes

- `lodev remove` keeps the volumes, and lists them.
- `lodev remove --volumes` deletes them, after asking.
- Switching to `none`, or dropping a folder from `volume_dirs`, also leaves the volume. `lodev start` lists those as unused.
