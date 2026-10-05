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

Release automation is configured; the first tagged release has not yet been published. Once available, download the Linux amd64 `.gz` archive and `SHA256SUMS` from the [latest GitHub Release](https://github.com/reinier-vegter/saltrtui/releases/latest). Until then, use the source build below or download the development executable from a successful [CI run](https://github.com/reinier-vegter/saltrtui/actions/workflows/ci.yml).

Place the release archive and manifest in a fresh directory containing only that release's downloads. Verify the compressed archive **before extracting** and install for your user:

```sh
set -e
sha256sum -c SHA256SUMS
gzip -d saltrtui_*_linux_amd64.gz
mkdir -p "$HOME/.local/bin"
install -m 755 saltrtui_*_linux_amd64 "$HOME/.local/bin/saltrtui"
"$HOME/.local/bin/saltrtui" --version
```

Checksums detect corrupted downloads; they are not an independent signature. Add `~/.local/bin` to your shell's `PATH` if necessary:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Persist that setting in your own shell configuration if desired. Restart the app after replacement; an already running process keeps its old version. If your shell still selects an older copy, check `command -v saltrtui` and clear its command cache (`hash -r` in Bash).

Updates are manual: repeat these steps for a newer release. Automatic update checks and in-app installation are not implemented. CI artifacts are development builds, not stable releases; unpack their ZIP and use `install -m 755 saltrtui-linux-amd64 "$HOME/.local/bin/saltrtui"`.

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
