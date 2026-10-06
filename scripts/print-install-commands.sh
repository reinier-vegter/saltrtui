#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ "$#" -ne 1 ]; then
    echo 'Usage: print-install-commands.sh vMAJOR.MINOR.PATCH' >&2
    exit 2
fi
version=$1
"$root/scripts/validate-release-version.sh" "$version"

archive="saltrtui_${version}_linux_amd64.gz"
url="https://github.com/reinier-vegter/saltrtui/releases/download/$version/$archive"

printf '%s\n' 'Linux amd64:'
printf 'd=$(mktemp -d) && trap '\''rm -rf "$d"'\'' EXIT && curl -fL '\''%s'\'' -o "$d/archive.gz" && gzip -dc "$d/archive.gz" > "$d/saltrtui" && test -s "$d/saltrtui" && sudo mkdir -p /usr/local/bin && test ! -L /usr/local/bin/saltrtui && sudo install -m 0755 "$d/saltrtui" /usr/local/bin/saltrtui\n' "$url"
