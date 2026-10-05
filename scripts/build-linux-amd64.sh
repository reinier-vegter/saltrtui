#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ "$#" -gt 1 ]; then
    echo 'Usage: build-linux-amd64.sh [dev|vMAJOR.MINOR.PATCH]' >&2
    exit 2
fi
version=${1-dev}
if [ "$version" != dev ]; then
    "$root/scripts/validate-release-version.sh" "$version"
fi
mkdir -p "$root/dist"
tmp_dir=$(mktemp -d "$root/dist/.saltrtui-linux-amd64.XXXXXX")
trap 'rm -r "$tmp_dir"' EXIT

cd "$root"
GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -buildvcs=false -ldflags="-X main.version=$version" \
    -o "$tmp_dir/saltrtui-linux-amd64" ./cmd/saltrtui
mv -f "$tmp_dir/saltrtui-linux-amd64" "$root/dist/saltrtui-linux-amd64"
printf 'Built %s\n' "$root/dist/saltrtui-linux-amd64"
