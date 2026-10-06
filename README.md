# saltrtui

[![CI](https://github.com/reinier-vegter/saltrtui/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/reinier-vegter/saltrtui/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/reinier-vegter/saltrtui?style=flat)](https://github.com/reinier-vegter/saltrtui/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/reinier-vegter/saltrtui?style=flat)](go.mod)
[![Platforms](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20(amd64%2C%20arm64)-1f6feb?style=flat)](https://github.com/reinier-vegter/saltrtui/releases/latest)
[![License](https://img.shields.io/github/license/reinier-vegter/saltrtui?style=flat)](LICENSE)

`saltrtui` is a keyboard-first Salt fleet workbench for operators on a Salt master.

## Features

- Browse accepted minions, observed presence, and minion-reported grains.
- Inspect cached jobs and returns; follow live master events and one minion's resource readings.
- Preview target IDs and run finite commands against a selected minion.
- Review and explicitly confirm minion key changes or a selected minion's highstate.

## Requirements

- Linux and macOS on amd64 or arm64 are supported **release targets**. The currently published `v0.0.2` predates this matrix and provides Linux amd64 only; the next stable release will publish all four assets. Linux artifacts are smoke-tested on Ubuntu 24.04 and Debian bookworm for both architectures. Cross-built macOS binaries require native validation on a Mac.
- Run on a Salt master host with `salt-key`, `salt-run`, and `salt` available to the current account.
- Use an operator account permitted to access the selected Salt master configuration, keys, logs, and job cache. Normal application use does not require elevation.

## Installation

The current stable release is [v0.0.2](https://github.com/reinier-vegter/saltrtui/releases/tag/v0.0.2). Its available Linux amd64 archive can be installed as follows. Future releases will provide the complete four-platform matrix below; do not use an unlisted filename before it is published.

### Linux

**Intel/AMD 64-bit:** [saltrtui_v0.0.2_linux_amd64.gz](https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.2/saltrtui_v0.0.2_linux_amd64.gz)

```sh
gunzip saltrtui_v0.0.2_linux_amd64.gz
test -s saltrtui_v0.0.2_linux_amd64
sudo mkdir -p /usr/local/bin
test ! -L /usr/local/bin/saltrtui
sudo install -m 0755 saltrtui_v0.0.2_linux_amd64 /usr/local/bin/saltrtui
```

Linux arm64 will be available in the next stable release.

### macOS

macOS archives will be available in the next stable release.

Confirm command resolution and the installed release:

```sh
command -v saltrtui
saltrtui --version
```

The command should resolve to `/usr/local/bin/saltrtui` and report `saltrtui v0.0.2`. If it does not, put `/usr/local/bin` on `PATH`, clear the shell command cache (for example, `hash -r` in Bash), and check again. Do not overwrite a package-managed executable; use its distribution channel or install the standalone release in a safe location instead.

### Update

Stable release installs check GitHub for a newer stable release in the background at most once per hour. When `vX.Y.Z available · U: update` appears in Fleet, press `U` to review the running and available versions and exact replacement path. Nothing downloads until you confirm **Update in place**. `Esc` or Cancel leaves without changing anything; a protected standalone installation may show the system terminal's sudo prompt for the narrowly scoped replacement. The updater replaces only the inspected executable in place, never changes `PATH` or other installations. Restart saltrtui after a successful update.

## Usage

Launch the installed binary on the Salt master:

```sh
saltrtui
saltrtui -config-dir /etc/salt
```

Press `?` in the application for keyboard help. Use `-no-alt-screen` if you need inline terminal output.

## Build from source

Use the Go version declared in `go.mod`.

```sh
scripts/verify.sh
```

The verification gate runs tests, vet, package builds, the fresh Linux amd64 development executable, and cross-builds all four release targets. `scripts/package-release.sh v1.2.3` creates four deterministic archives and `SHA256SUMS` under `dist/release/v1.2.3/`; it does not create a tag or publish a release. Linux cross-build and container checks do not prove native macOS behavior or live Salt compatibility.

## License

Released under the [MIT License](LICENSE).
