---
title: "Customizing the web image"
description: "Extend the project web image with your own Dockerfile."
---

Add your own build steps to a project's web image, with a `Dockerfile`.

- Put `Dockerfile*` files in `.lodev/web-images/`.
- They are added in name order, after the generated steps.
- `*.example` files are ignored.
- Write plain Dockerfile instructions. Do not add a `FROM`.

```dockerfile
# .lodev/web-images/Dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends poppler-utils && rm -rf /var/lib/apt/lists/*
```

- Editing a file rebuilds the project image on the next `lodev start` or `lodev restart`.
- Leaving the files alone builds nothing.
- `lodev create` scaffolds `Dockerfile.example`.
