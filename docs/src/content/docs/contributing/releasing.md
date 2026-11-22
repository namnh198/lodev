---
title: "Releasing"
description: "How a release is cut, signed and published."
---

Maintainer guide: how a release is cut, signed and published.

## CI

`.github/workflows/ci.yml` runs `go vet`, `go test` and `make build` on:

- pushes to `main`
- pull requests

## Cutting a stable release

Push a tag:

```bash
git tag v1.2.3 && git push origin v1.2.3
```

`.github/workflows/release.yml` then runs `make packages` and creates the GitHub release with:

- the archives
- the `.deb` files
- `checksums.txt`
- `lodev.rb`
- `PKGBUILD` and `.SRCINFO`

Notes:

- A tag with `-` in it, like `v1.3.0-rc1`, is marked pre-release.
- A stable tag also commits `lodev.rb` to `Formula/lodev.rb` in `namnh198/homebrew-tap` ("feat: release lodev vX.Y.Z").
  - That needs the repository secret `HOMEBREW_TAP_TOKEN`: a token with Contents read/write on the tap repo.
- The rest is manual: bump `version` in `flake.nix`.
- The `PKGBUILD` is only published as a release file, not on the AUR.

## Base PHP image

A stable tag also builds the PHP image for amd64 and arm64 (native runners). It pushes the image to `ghcr.io/namnh198/lodev-webserver:<tag>`. That lodev version pulls it instead of building it.

After the first push, make the package public once:

1. GitHub → Packages → `lodev-webserver`.
2. Package settings → Change visibility → Public.

Until then, lodev's pull fails and it builds locally.

## Nightly

`.github/workflows/nightly.yml` replaces the `nightly` pre-release on every push to `main`:

- It runs `make dist VER=nightly CHANNEL=nightly`.
- It is never marked latest, so stable users are not affected.

## Signed releases

Both workflows sign `dist/checksums.txt`:

- Key: ed25519, via `go run ./tools/sign -key-env LODEV_SIGNING_KEY dist/checksums.txt`.
- Output: uploaded as `checksums.txt.sig`.
- A missing `LODEV_SIGNING_KEY` secret fails the job, so no release goes out unsigned.

How `lodev self-upgrade` uses it:

- It checks the signature against the public key built into lodev (`releasePublicKey` in `internal/update/download.go`) before it trusts `checksums.txt`.
- If the signature is missing or wrong, it installs nothing.
- This includes `nightly` and every older tag.
- Releases made before signing cannot be installed with `self-upgrade`. Use `install.sh` or your package manager for them.

### One-time setup and key rotation

Both use the same steps:

1. Run `go run ./tools/sign -genkey`. It prints `private:` and `public:`.
2. Store the private key as the repository secret `LODEV_SIGNING_KEY`. Keep it nowhere else.
3. Paste the public key into `releasePublicKey` and commit it:
   - before pushing `main`, since nightly signs on every push
   - and before tagging a release

### Rotating the key

- It means a new lodev release.
- A build trusts only the key it shipped with.
- Users on an older build must upgrade with `install.sh` or their package manager.
- Releases signed with the old key stop verifying in new builds.
