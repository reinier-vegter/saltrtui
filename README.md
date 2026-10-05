# saltrtui

`saltrtui` is a keyboard-first Salt fleet workbench for operators on a Salt master.

## Features

- Browse accepted minions, observed presence, and minion-reported grains.
- Inspect cached jobs and returns; follow live master events and one minion's resource readings.
- Preview target IDs and run finite commands against a selected minion.
- Review and explicitly confirm minion key changes or a selected minion's highstate.

## Requirements

- Linux amd64 is the currently verified build target.
- Run on a Salt master host with `salt-key`, `salt-run`, and `salt` available to the current account.
- Use an operator account permitted to access the selected Salt master configuration, keys, logs, and job cache. The TUI does not elevate privileges.

## Versioning and installation

There is no tagged GitHub Release yet. When one is published, obtain the Linux amd64 binary from [GitHub Releases](https://github.com/reinier-vegter/saltrtui/releases) and install it on your `PATH`. Until then, use the source build instructions below. A version-reporting flag is not yet available.

## Run

Launch the installed binary on the Salt master:

```sh
saltrtui
saltrtui -config-dir /etc/salt
```

Press `?` in the application for keyboard help. Use `-no-alt-screen` if you need inline terminal output.

## Updates

Automatic update checks and in-app installation are not implemented yet. When a newer GitHub Release becomes available, install its binary manually.

## Build from source

Use the Go version declared in `go.mod`. The verification script runs tests, vet, package builds, and a Linux amd64 build:

```sh
scripts/verify.sh
```

To build only the Linux amd64 executable, run `scripts/build-linux-amd64.sh`. Output is written to the ignored `dist/saltrtui-linux-amd64` path.
