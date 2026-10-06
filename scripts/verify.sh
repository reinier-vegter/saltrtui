#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

go test ./...
go vet ./...
go build ./...
"$root/scripts/build-linux-amd64.sh"
"$root/scripts/build-targets.sh"
