#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ "$#" -ne 1 ]; then
    echo 'Usage: package-release.sh vMAJOR.MINOR.PATCH' >&2
    exit 2
fi
version=$1
"$root/scripts/validate-release-version.sh" "$version"
if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != x86_64 ]; then
    echo 'Release packaging requires a Linux amd64 host for executable verification' >&2
    exit 1
fi
destination="$root/dist/release/$version"
if [ -e "$destination" ] || [ -L "$destination" ]; then
    echo "Release output already exists: $destination (inspect it before removing or retrying)" >&2
    exit 1
fi
mkdir -p "$root/dist/release"
stage=$(mktemp -d "$root/dist/release/.package.XXXXXX")
trap 'rm -r "$stage"' EXIT

"$root/scripts/build-targets.sh" "$version"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${target%/*}
    arch=${target#*/}
    binary="$root/dist/targets/$version/saltrtui_${version}_${os}_${arch}"
    archive="saltrtui_${version}_${os}_${arch}.gz"
    test -s "$binary"
    go version -m "$binary" > "$stage/build-metadata"
    grep -q "GOOS=$os" "$stage/build-metadata"
    grep -q "GOARCH=$arch" "$stage/build-metadata"
    grep -q 'CGO_ENABLED=0' "$stage/build-metadata"
    if [ "$arch" = amd64 ]; then grep -q 'GOAMD64=v1' "$stage/build-metadata"; fi
    if [ "$arch" = arm64 ]; then grep -q 'GOARM64=v8.0' "$stage/build-metadata"; fi
    if [ "$os" = linux ]; then
        # The packaging host is Linux amd64. Runtime validation for linux/arm64
        # happens under emulation in the release workflow below; executing it
        # here would fail before that matrix can run.
        if [ "$arch" = amd64 ]; then
            test "$("$binary" --version)" = "saltrtui $version"
        fi
        ! readelf -l "$binary" | grep -q 'Requesting program interpreter'
        ! readelf -d "$binary" | grep -q 'NEEDED'
    fi
    gzip -9 -n -c "$binary" > "$stage/$archive"
    gzip -t "$stage/$archive"
    rm "$stage/build-metadata"
done
cd "$stage"
sha256sum saltrtui_*.gz > SHA256SUMS
sha256sum -c SHA256SUMS
for archive in saltrtui_*.gz; do
    gzip -dc "$archive" > extracted
    test -s extracted
    rm extracted
done
cd "$root"
# Linux mv -T refuses to merge into an existing directory. Publish as one set.
mv -T "$stage" "$destination"
trap - EXIT
printf 'Packaged %s\n' "$destination"
