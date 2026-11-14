---
paths:
  - "tools/sign/**"
  - "internal/update/**"
  - "cmd/selfupgrade.go"
  - ".github/workflows/**"
  - "Makefile"
  - "packaging/**"
  - "install.sh"
---

# Releases and self-upgrade

- The ed25519 private signing key lives only in the `LODEV_SIGNING_KEY` repository secret. The
  user generates and stores it; an agent only ever handles the public key (`releasePublicKey`
  in `internal/update/download.go`).
- `self-upgrade` verifies `checksums.txt.sig` before it trusts `checksums.txt`, and installs
  nothing when the signature is missing or wrong. Keep that order.
- A release-path change is checked with `make packages` (what CI and `release.yml` run), not
  only `go test`.
- Tags, pushes and GitHub releases are the user's to trigger.
