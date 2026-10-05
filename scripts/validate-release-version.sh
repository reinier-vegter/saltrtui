#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
    echo 'Usage: validate-release-version.sh vMAJOR.MINOR.PATCH' >&2
    exit 2
fi
# Reject controls/newlines before grep: its anchors apply to individual lines.
case "$1" in
    ''|*[!v0-9.]*)
        echo 'Invalid release version: expected stable vMAJOR.MINOR.PATCH' >&2
        exit 2
        ;;
esac
if ! printf '%s\n' "$1" | LC_ALL=C grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
    echo 'Invalid release version: expected stable vMAJOR.MINOR.PATCH without leading zeros' >&2
    exit 2
fi
