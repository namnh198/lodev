---
title: "Releasing"
description: "How a release is cut, signed and published."
---

CI (`.github/workflows/ci.yml`) runs `go vet`, `go test` and `make build` on pushes to `main` and
on pull requests. Pushing a tag releases:

```bash
git tag v1.2.3 && git push origin v1.2.3
```

`.github/workflows/release.yml` runs `make packages` and creates the GitHub release with the
archives, `.deb` files, `checksums.txt`, `lodev.rb`, `PKGBUILD` and `.SRCINFO` (a tag with `-`,
like `v1.3.0-rc1`, is marked pre-release). A stable tag also commits `lodev.rb` to
`Formula/lodev.rb` in `namnh198/homebrew-tap` ("feat: release lodev vX.Y.Z"); that needs a repository
secret `HOMEBREW_TAP_TOKEN`, a token with Contents read/write on the tap repo. The rest is manual:
bump `version` in `flake.nix` (the `PKGBUILD` is only published as a release file, not on the AUR).

A stable tag also builds the PHP image for amd64 and arm64 (native runners) and pushes it to
`ghcr.io/namnh198/lodev-webserver:<tag>`; that lodev version pulls it instead of building it.
After the first push, make the package public once (GitHub → Packages → `lodev-webserver` →
Package settings → Change visibility → Public); until then lodev's pull fails and it builds locally.

`.github/workflows/nightly.yml` replaces the `nightly` pre-release on every push to `main`
(`make dist VER=nightly CHANNEL=nightly`); it is never marked latest, so stable users are not affected.

## Signed releases

Both workflows sign `dist/checksums.txt` with an ed25519 key (`go run ./tools/sign -key-env
LODEV_SIGNING_KEY dist/checksums.txt`) and upload the result as `checksums.txt.sig`; a missing
`LODEV_SIGNING_KEY` secret fails the job, so no release goes out unsigned. `lodev self-upgrade`
checks that signature against the public key built into lodev (`releasePublicKey` in
`internal/update/download.go`) before it trusts `checksums.txt`, and installs nothing if it is
missing or wrong. That includes `nightly` and every older tag: releases made before signing
cannot be installed with `self-upgrade`; use `install.sh` or your package manager for them.

One-time setup, and key rotation (same steps):

1. `go run ./tools/sign -genkey` prints `private:` and `public:`.
2. Store the private key as the repository secret `LODEV_SIGNING_KEY`; keep it nowhere else.
3. Paste the public key into `releasePublicKey` and commit it, before pushing `main` (nightly
   signs on every push) and before tagging a release.

Rotating the key means a new lodev release: a build only trusts the key it shipped with, so
users on an older build must upgrade with `install.sh` or their package manager (and releases
signed with the old key stop verifying in new builds).
