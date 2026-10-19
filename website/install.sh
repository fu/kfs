#!/bin/sh
# Install kfs: https://github.com/fu/kfs
#
#   curl -fsSL https://getkfs.org/install.sh | sh
#
# Picks the build for this machine (macOS or Linux, amd64 or arm64), checks
# it against the release's checksums and puts kfs into /usr/local/bin if
# that is writable, else ~/.local/bin. Settings:
#
#   KFS_VERSION=v0.5.2        a release other than the latest
#   KFS_INSTALL_DIR=~/bin     where kfs goes
#
# Windows: download kfs from https://github.com/fu/kfs/releases instead.

set -eu

repo="fu/kfs"

say() { printf 'kfs: %s\n' "$*"; }
die() { printf 'kfs: %s\n' "$*" >&2; exit 1; }

# fetch URL FILE downloads with curl or wget.
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL -o "$2" "$1"; }
  latest() { curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
  latest() { wget -q -S --spider "https://github.com/$repo/releases/latest" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1; }
else
  die "needs curl or wget"
fi

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "no build for $(uname -s) here: get one from https://github.com/$repo/releases" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "no build for $(uname -m): get one from https://github.com/$repo/releases" ;;
esac

# An Intel shell on Apple silicon (Rosetta) says x86_64; the arm64 build is the one to run.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
  arch=arm64
fi

version="${KFS_VERSION:-}"
if [ -z "$version" ]; then
  version="$(latest | tr -d '\r')"
  version="${version##*/}"
fi
case "$version" in
  v*) ;;
  *) die "could not find the latest release (got '$version'); set KFS_VERSION, e.g. KFS_VERSION=v0.5.2" ;;
esac

archive="kfs_${version#v}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$version"

tmp="$(mktemp -d 2>/dev/null || mktemp -d -t kfs)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "downloading $version for $os/$arch"
fetch "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"

want="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || die "$archive is not in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$tmp/$archive" | awk '{ print $1 }')"
else
  got="$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')"
fi
[ "$got" = "$want" ] || die "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" kfs

dir="${KFS_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  else
    dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$dir"
mv -f "$tmp/kfs" "$dir/kfs"
chmod 755 "$dir/kfs"

say "installed $dir/kfs ($version)"
case ":$PATH:" in
  *":$dir:"*) say "run it: kfs -n <namespace>" ;;
  *) say "$dir is not on your PATH; add it, e.g.: export PATH=\"$dir:\$PATH\"" ;;
esac
