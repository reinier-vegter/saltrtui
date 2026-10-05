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

"$root/scripts/build-linux-amd64.sh" "$version"
binary="$root/dist/saltrtui-linux-amd64"
test "$("$binary" --version)" = "saltrtui $version"
go version -m "$binary" > "$stage/build-metadata"
grep -q 'GOOS=linux' "$stage/build-metadata"
grep -q 'GOARCH=amd64' "$stage/build-metadata"
grep -q 'CGO_ENABLED=0' "$stage/build-metadata"

archive="saltrtui_${version}_linux_amd64.gz"
gzip -n -c "$binary" > "$stage/$archive"
gzip -t "$stage/$archive"
cd "$stage"
sha256sum "$archive" > SHA256SUMS
sha256sum -c SHA256SUMS
gzip -dc "$archive" > extracted
test -s extracted
chmod 755 extracted
test "$(./extracted --version)" = "saltrtui $version"
rm extracted build-metadata
cd "$root"
# Linux mv -T refuses to merge into an existing directory. Publish as one set.
mv -T "$stage" "$destination"
trap - EXIT
printf 'Packaged %s\n' "$destination"
