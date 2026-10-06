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

<!-- release-installation:start -->

The current stable release is [v0.0.5](https://github.com/reinier-vegter/saltrtui/releases/tag/v0.0.5). Choose the archive for your operating system and architecture. These commands require `curl`, `gunzip`, and `sudo`.

### Linux

**Intel/AMD 64-bit (`x86_64`):** [saltrtui_v0.0.5_linux_amd64.gz](https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_linux_amd64.gz)

```sh
sudo mkdir -p /usr/local/bin
test ! -L /usr/local/bin/saltrtui
curl -fsSL 'https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_linux_amd64.gz' | gunzip | sudo install -m 0755 /dev/stdin /usr/local/bin/saltrtui
```

**ARM 64-bit (`aarch64` / `arm64`):** [saltrtui_v0.0.5_linux_arm64.gz](https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_linux_arm64.gz)

```sh
sudo mkdir -p /usr/local/bin
test ! -L /usr/local/bin/saltrtui
curl -fsSL 'https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_linux_arm64.gz' | gunzip | sudo install -m 0755 /dev/stdin /usr/local/bin/saltrtui
```

### macOS

**Intel Mac:** [saltrtui_v0.0.5_darwin_amd64.gz](https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_darwin_amd64.gz)

```sh
sudo mkdir -p /usr/local/bin
test ! -L /usr/local/bin/saltrtui
curl -fsSL 'https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_darwin_amd64.gz' | gunzip | sudo install -m 0755 /dev/stdin /usr/local/bin/saltrtui
```

**Apple Silicon Mac (M-series):** [saltrtui_v0.0.5_darwin_arm64.gz](https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_darwin_arm64.gz)

```sh
sudo mkdir -p /usr/local/bin
test ! -L /usr/local/bin/saltrtui
curl -fsSL 'https://github.com/reinier-vegter/saltrtui/releases/download/v0.0.5/saltrtui_v0.0.5_darwin_arm64.gz' | gunzip | sudo install -m 0755 /dev/stdin /usr/local/bin/saltrtui
```

Confirm command resolution and the installed release:

```sh
command -v saltrtui
saltrtui --version
```

The command should resolve to `/usr/local/bin/saltrtui` and report `saltrtui v0.0.5`. If it does not, put `/usr/local/bin` on `PATH`, clear the shell command cache (for example, `hash -r` in Bash), and check again. Do not overwrite a package-managed executable; use its distribution channel or install the standalone release in a safe location instead.

<!-- release-installation:end -->

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
