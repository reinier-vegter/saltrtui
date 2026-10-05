#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$root/dist"
tmp_dir=$(mktemp -d "$root/dist/.saltrtui-linux-amd64.XXXXXX")
trap 'rm -r "$tmp_dir"' EXIT

cd "$root"
GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -buildvcs=false -o "$tmp_dir/saltrtui-linux-amd64" ./cmd/saltrtui
mv -f "$tmp_dir/saltrtui-linux-amd64" "$root/dist/saltrtui-linux-amd64"
printf 'Built %s\n' "$root/dist/saltrtui-linux-amd64"
