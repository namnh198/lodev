---
title: "Performance mode"
description: "Trade file-sync safety for speed on bind mounts."
---

With `performance_mode: bind` (the default on macOS, where bind mounts are slow; Linux defaults to `none`),
some project folders live in Docker volumes instead of the bind mount. Set it per project
(`lodev config --performance-mode=bind|none`) or for all (`lodev config-global --performance-mode=...`);
`""` goes back to the next level (project, then global, then the OS default).

| Type | Folders in volumes |
|---|---|
| magento | `var`, `generated`, `node_modules`* |
| laravel, wordpress, php | `node_modules`* |

\* only when `package.json` exists at the project root.

Laravel's `storage` stays on the bind mount (it holds real data); add it to `volume_dirs` if you want it
in a volume. `volume_dirs` adds your own folders (`lodev config --add-volume-dirs=pub/media`): relative
paths of `A-Z a-z 0-9 . _ -` segments, not `.lodev`, no folder inside another one in volumes, and no two
that map to the same volume name (`lodev-<name>-<folder with / as ->`).

- `var`, `generated` and `node_modules` start empty in their volume. Every `volume_dirs` folder is copied
  from the host once, when its volume is created.
- The host no longer sees what the container has in these folders: read logs with `lodev logs` or
  `lodev exec tail`, and host-side `npm` or IDE indexing of `generated/` need `performance_mode: none`.
- `lodev start` creates the volumes (labelled `lodev.project=<name>`) before the containers and prints
  what each folder got. A folder that is a symlink on the host is skipped with a warning.
- Files the container writes into a seeded folder (e.g. uploads in `pub/media`) live only in its volume.
  `lodev remove` keeps the volumes and lists them; `lodev remove --volumes` deletes them after asking.
  Switching to `none` or dropping a folder from `volume_dirs` also leaves the volume: `start` lists
  those as unused.
- `lodev describe` shows the effective mode and folders.
