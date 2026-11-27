#!/bin/sh
# Fill the package templates for one release.
# usage: packaging/render.sh <version without v> <SHA256SUMS file> <out dir>
# Writes <out>/scoop/spotui.json, <out>/aur/PKGBUILD and .SRCINFO,
# <out>/homebrew/spotui.rb.
set -eu
version=$1 sums=$2 out=$3
here=$(dirname "$0")
sha() { awk -v f="$1" '$2 == f || $2 == "*"f { print $1 }' "$sums"; }
linux=$(sha spotui-linux-amd64.tar.gz)
windows=$(sha spotui-windows-amd64.zip)
[ -n "$linux" ] && [ -n "$windows" ] || { echo "render.sh: hashes missing in $sums" >&2; exit 1; }
# pacman versions can't hold '-': 0.4.0-rc1 becomes 0.4.0_rc1.
pkgver=$(printf %s "$version" | tr - _)
fill() {
	mkdir -p "$(dirname "$2")"
	sed -e "s/@VERSION@/$version/g" -e "s/@PKGVER@/$pkgver/g" \
		-e "s/@SHA_LINUX@/$linux/g" -e "s/@SHA_WINDOWS@/$windows/g" "$1" >"$2"
}
fill "$here/scoop.json.in" "$out/scoop/spotui.json"
fill "$here/aur/PKGBUILD.in" "$out/aur/PKGBUILD"
fill "$here/aur/SRCINFO.in" "$out/aur/.SRCINFO"
fill "$here/homebrew/spotui.rb.in" "$out/homebrew/spotui.rb"
