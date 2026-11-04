#!/bin/sh
# Install lodev and its bash/zsh/fish completions from a GitHub release.
#   curl -fsSL https://raw.githubusercontent.com/namnh198/lodev/main/install.sh | sh
# Environment: VERSION (default: latest; edge = pre-release built from main), PREFIX (default: /usr/local), REPO, BASE_URL.
set -eu

REPO=${REPO:-namnh198/lodev}
PREFIX=${PREFIX:-/usr/local}
BASE_URL=${BASE_URL:-https://github.com/$REPO/releases/download}

fail() {
	echo "lodev install: $*" >&2
	exit 1
}

os=${LODEV_UNAME_S:-$(uname -s)}
case $os in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported OS $os (lodev runs on Linux and macOS)" ;;
esac
arch=${LODEV_UNAME_M:-$(uname -m)}
case $arch in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported architecture $arch" ;;
esac

command -v curl >/dev/null || fail "curl is required"
if [ -z "${VERSION:-}" ]; then
	VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
		fail "cannot find the latest release of $REPO"
	VERSION=${VERSION##*/}
fi
case $VERSION in v* | edge) ;; *) VERSION=v$VERSION ;; esac
ver=${VERSION#v}
name=lodev_${ver}_${os}_${arch}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "Downloading lodev $VERSION ($os/$arch)"
curl -fsSL -o "$tmp/$name.tar.gz" "$BASE_URL/$VERSION/$name.tar.gz" || fail "download of $name.tar.gz failed"
curl -fsSL -o "$tmp/checksums.txt" "$BASE_URL/$VERSION/checksums.txt" || fail "download of checksums.txt failed"

want=$(awk -v f="$name.tar.gz" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "$name.tar.gz is not listed in checksums.txt"
if command -v sha256sum >/dev/null; then
	got=$(sha256sum "$tmp/$name.tar.gz" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$name.tar.gz" | awk '{ print $1 }')
fi
[ "$got" = "$want" ] || fail "checksum mismatch for $name.tar.gz (want $want, got $got)"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
src=$tmp/$name

# sudo only when the prefix is not writable by us.
sudo=
probe=$PREFIX
while [ ! -d "$probe" ]; do probe=$(dirname "$probe"); done
if [ ! -w "$probe" ]; then
	command -v sudo >/dev/null || fail "$PREFIX is not writable and sudo is missing; set PREFIX=\$HOME/.local"
	sudo=sudo
fi

$sudo mkdir -p "$PREFIX/bin" "$PREFIX/share/bash-completion/completions" \
	"$PREFIX/share/zsh/site-functions" "$PREFIX/share/fish/vendor_completions.d"
# A new file renamed into place, not cp into the old one: that would keep the old file's
# com.apple.quarantine (e.g. from a browser download) and can kill a running signed binary on macOS.
$sudo install -m 0755 "$src/lodev" "$PREFIX/bin/.lodev-install-new"
$sudo mv -f "$PREFIX/bin/.lodev-install-new" "$PREFIX/bin/lodev"
if [ "$os" = darwin ]; then
	$sudo xattr -d com.apple.quarantine "$PREFIX/bin/lodev" 2>/dev/null || true
fi
$sudo cp "$src/completions/lodev.bash" "$PREFIX/share/bash-completion/completions/lodev"
$sudo cp "$src/completions/_lodev" "$PREFIX/share/zsh/site-functions/_lodev"
$sudo cp "$src/completions/lodev.fish" "$PREFIX/share/fish/vendor_completions.d/lodev.fish"
if [ -f "$src/LICENSE" ]; then # releases before the MIT license have none
	$sudo mkdir -p "$PREFIX/share/doc/lodev"
	$sudo cp "$src/LICENSE" "$PREFIX/share/doc/lodev/LICENSE"
fi

echo "Installed lodev $VERSION to $PREFIX/bin/lodev (with bash, zsh and fish completions)"

case ":$PATH:" in
*":$PREFIX/bin:"*) ;;
*)
	cat <<EOF

$PREFIX/bin is not on your PATH. Add it:
  bash (~/.bashrc), zsh (~/.zshrc):  export PATH="$PREFIX/bin:\$PATH"
  fish:                              fish_add_path "$PREFIX/bin"
EOF
	;;
esac

# Shells only look for completions under the standard prefixes.
case $PREFIX in
/usr/local | /usr) ;;
*)
	cat <<EOF

Completions are in $PREFIX/share, which your shell does not read by default. Load them:
  bash (~/.bashrc):                  source "$PREFIX/share/bash-completion/completions/lodev"
  zsh  (~/.zshrc, before compinit):  fpath=("$PREFIX/share/zsh/site-functions" \$fpath)
  fish (~/.config/fish/config.fish): set -p fish_complete_path "$PREFIX/share/fish/vendor_completions.d"
EOF
	;;
esac
