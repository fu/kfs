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
#   NO_COLOR=1                no colors
#
# Windows: download kfs from https://github.com/fu/kfs/releases instead.

set -eu

repo="fu/kfs"

# Colors and the progress bar only on a terminal (stderr is one even when
# the script itself comes from a pipe).
if [ -t 2 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-dumb}" != dumb ]; then
  cyan="$(printf '\033[38;5;45m')" dim="$(printf '\033[2m')" bold="$(printf '\033[1m')" off="$(printf '\033[0m')"
else
  cyan="" dim="" bold="" off=""
fi
tty=false
[ -t 2 ] && tty=true

say() { printf '%s\n' "$*" >&2; }
step() { printf '%s%s%s %s\n' "$cyan" "$1" "$off" "$2" >&2; }
ok() { printf '      %sok%s %s\n' "$dim" "$off" "$*" >&2; }
die() { printf 'kfs: %s\n' "$*" >&2; exit 1; }

logo() {
  printf '%s' "$cyan" >&2
  cat >&2 <<'EOF'

 _________.       _________.
 \__     /_______/    ____/_______.
  /     /     _/_     __/     ___/
_/     .      \/     /   \      \
\_______\      \__. /___________/
         \_______/ /
           \______/
EOF
  printf '%s\n' "$off" >&2
}

# fetch URL FILE downloads quietly, download URL FILE with a progress bar on
# a terminal; latest prints the URL the latest release redirects to.
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL -o "$2" "$1"; }
  if $tty; then
    download() { curl -fL --progress-bar -o "$2" "$1"; }
  else
    download() { fetch "$@"; }
  fi
  latest() { curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
  if $tty && wget --help 2>&1 | grep -q -- --show-progress; then
    # run in the file's folder so the bar shows its name, not the temp path
    download() { (cd "$(dirname "$2")" && wget -q --show-progress -O "$(basename "$2")" "$1"); } # GNU wget
  elif $tty; then
    download() { (cd "$(dirname "$2")" && wget -O "$(basename "$2")" "$1"); } # BusyBox wget draws its own bar
  else
    download() { fetch "$@"; }
  fi
  latest() { wget -q -S --spider "https://github.com/$repo/releases/latest" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1; }
else
  die "needs curl or wget"
fi

logo

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
  step "[1/4]" "finding the latest release"
  version="$(latest | tr -d '\r')"
  version="${version##*/}"
else
  step "[1/4]" "release $version (KFS_VERSION)"
fi
case "$version" in
  v*) ;;
  *) die "could not find the latest release (got '$version'); set KFS_VERSION, e.g. KFS_VERSION=v0.5.2" ;;
esac
ok "$version for $os/$arch"

archive="kfs_${version#v}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$version"

tmp="$(mktemp -d 2>/dev/null || mktemp -d -t kfs)"
trap 'rm -rf "$tmp"' EXIT INT TERM

step "[2/4]" "downloading $archive"
download "$base/$archive" "$tmp/$archive" || die "download failed: $base/$archive"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"
size="$(wc -c <"$tmp/$archive" | awk '{ printf "%.1f MB", $1 / 1048576 }')"
ok "$size"

step "[3/4]" "checking it against the release checksums"
want="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || die "$archive is not in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$tmp/$archive" | awk '{ print $1 }')"
else
  got="$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')"
fi
[ "$got" = "$want" ] || die "checksum mismatch for $archive"
ok "sha256 $(printf '%s' "$got" | cut -c1-12)…"

dir="${KFS_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  else
    dir="$HOME/.local/bin"
  fi
fi

step "[4/4]" "installing into $dir"
tar -xzf "$tmp/$archive" -C "$tmp" kfs
mkdir -p "$dir"
mv -f "$tmp/kfs" "$dir/kfs"
chmod 755 "$dir/kfs"
ok "$dir/kfs"

say ""
say "${bold}kfs $version is installed.${off}"
case ":$PATH:" in
  *":$dir:"*) say "Run it: ${cyan}kfs -n <namespace>${off}" ;;
  *) say "$dir is not on your PATH yet; add it, e.g.:" && say "  ${cyan}export PATH=\"$dir:\$PATH\"${off}" ;;
esac
