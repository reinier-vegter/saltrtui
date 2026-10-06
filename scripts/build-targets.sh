#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ "$#" -gt 1 ]; then
    echo 'Usage: build-targets.sh [dev|vMAJOR.MINOR.PATCH]' >&2
    exit 2
fi
version=${1-dev}
if [ "$version" != dev ]; then
    "$root/scripts/validate-release-version.sh" "$version"
fi

out="$root/dist/targets/$version"
if [ -e "$out" ] || [ -L "$out" ]; then
    rm -rf "$out"
fi
mkdir -p "$out"
trap 'rm -rf "$out"' EXIT

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${target%/*}
    arch=${target#*/}
    name="saltrtui_${version}_${os}_${arch}"
    GOWORK=off CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 \
        go build -trimpath -buildvcs=false -ldflags="-X main.version=$version" \
        -o "$out/$name" ./cmd/saltrtui
done

trap - EXIT
printf 'Built targets in %s\n' "$out"
