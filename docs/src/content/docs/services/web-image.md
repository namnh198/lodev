---
title: "Customizing the web image"
description: "Extend the project web image with your own Dockerfile."
---

Put `Dockerfile*` files in `.lodev/web-images/` (added in name order after the generated
steps; `*.example` files are ignored). Plain Dockerfile instructions, no `FROM`:

```dockerfile
# .lodev/web-images/Dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends poppler-utils && rm -rf /var/lib/apt/lists/*
```

Editing a file rebuilds the project image on the next `lodev start` / `restart`; leaving it
alone builds nothing. `lodev create` scaffolds `Dockerfile.example`.
