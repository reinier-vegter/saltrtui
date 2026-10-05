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

printf '%s\n' 'Ubuntu 24.04 (Linux amd64):'
printf 'mkdir -p "$HOME/.local/bin" && (d=$(mktemp -d "$HOME/.local/bin/.saltrtui.XXXXXX") && trap '\''rm -rf "$d"'\'' EXIT && curl -fL '\''%s'\'' -o "$d/archive.gz" && gzip -dc "$d/archive.gz" > "$d/saltrtui" && chmod 755 "$d/saltrtui" && mv -fT "$d/saltrtui" "$HOME/.local/bin/saltrtui")\n' "$url"
printf '\n%s\n' 'Debian bookworm (Linux amd64):'
printf 'mkdir -p "$HOME/.local/bin" && (d=$(mktemp -d "$HOME/.local/bin/.saltrtui.XXXXXX") && trap '\''rm -rf "$d"'\'' EXIT && curl -fL '\''%s'\'' -o "$d/archive.gz" && gzip -dc "$d/archive.gz" > "$d/saltrtui" && chmod 755 "$d/saltrtui" && mv -fT "$d/saltrtui" "$HOME/.local/bin/saltrtui")\n' "$url"
