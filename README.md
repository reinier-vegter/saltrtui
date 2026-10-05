# saltrtui

[![CI](https://github.com/reinier-vegter/saltrtui/actions/workflows/ci.yml/badge.svg)](https://github.com/reinier-vegter/saltrtui/actions/workflows/ci.yml)

`saltrtui` is a keyboard-first Salt fleet workbench for operators on a Salt master.

## Features

- Browse accepted minions, observed presence, and minion-reported grains.
- Inspect cached jobs and returns; follow live master events and one minion's resource readings.
- Preview target IDs and run finite commands against a selected minion.
- Review and explicitly confirm minion key changes or a selected minion's highstate.

## Requirements

- Linux amd64 is the only supported binary/release target.
- Run on a Salt master host with `salt-key`, `salt-run`, and `salt` available to the current account.
- Use an operator account permitted to access the selected Salt master configuration, keys, logs, and job cache. The TUI does not elevate privileges.

## Versioning

Stable releases use `vMAJOR.MINOR.PATCH` tags. Run `saltrtui --version` to check your installed version; source/development builds report `dev`.

## Install and update

Release automation is configured; the first tagged release has not yet been published. When available, open the [latest GitHub Release](https://github.com/reinier-vegter/saltrtui/releases/latest), then copy the Ubuntu 24.04 or Debian bookworm install command from the successful [release workflow run summary](https://github.com/reinier-vegter/saltrtui/actions/workflows/release.yml). Both use the Linux amd64 binary and install it to `~/.local/bin/saltrtui`. The command requires `curl`, `gzip`, and GNU coreutils. Until the first release, use the source build below.

Add `~/.local/bin` to your shell's `PATH` if necessary:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Persist that setting in your own shell configuration if desired. Restart the app after installation or replacement; an already running process keeps its old version.

Updates are manual: copy the install command from each new release's workflow summary. Automatic update checks and in-app installation are not implemented. CI artifacts are development builds, not stable releases.

## Run

Launch the installed binary on the Salt master:

```sh
saltrtui
saltrtui -config-dir /etc/salt
```

Press `?` in the application for keyboard help. Use `-no-alt-screen` if you need inline terminal output.

## Build from source

Use the Go version declared in `go.mod`. The verification script runs tests, vet, package builds, and a Linux amd64 build:

```sh
scripts/verify.sh
```

To build only the Linux amd64 executable, run `scripts/build-linux-amd64.sh`. Output is written to the ignored `dist/saltrtui-linux-amd64` path. An optional stable version argument embeds a release tag instead of `dev`:

```sh
scripts/build-linux-amd64.sh v1.2.3
dist/saltrtui-linux-amd64 --version
```

On Linux amd64, `scripts/package-release.sh v1.2.3` creates a verified gzip archive and `SHA256SUMS` under `dist/release/v1.2.3/`. This example version does not create a Git tag or publish anything. Packaging refuses an existing output directory rather than overwriting it.

Maintainers publish by pushing an intentional stable tag in this public repository after CI passes. The release workflow independently verifies the tagged commit and smoke-tests the binary on Ubuntu and Debian before uploading assets to a draft and publishing it. Existing releases/drafts are never overwritten; inspect and explicitly clean up an incomplete draft before rerunning publication. Go checks and version smoke tests do not validate live Salt behavior.
