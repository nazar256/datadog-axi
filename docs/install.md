# Install and release guide

## Recommended: install the latest release

The installer downloads the right release archive for your platform and verifies its SHA256 checksum before installing `ddog`.

```bash
curl -fsSL https://github.com/nazar256/datadog-cli/releases/latest/download/install.sh | sh
```

Install a specific version:

```bash
curl -fsSL https://github.com/nazar256/datadog-cli/releases/latest/download/install.sh | sh -s -- --version v1.0.1
```

Install into a specific directory:

```bash
curl -fsSL https://github.com/nazar256/datadog-cli/releases/latest/download/install.sh | sh -s -- --install-dir "$HOME/.local/bin"
```

## Manual Linux install with checksum verification

Example for `linux/amd64`:

```bash
VERSION=v1.0.1
ARCHIVE="datadog-cli_${VERSION}_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/nazar256/datadog-cli/releases/download/${VERSION}/${ARCHIVE}"
curl -fsSLO "https://github.com/nazar256/datadog-cli/releases/download/${VERSION}/datadog-cli_${VERSION}_checksums.txt"
grep "  ${ARCHIVE}$" "datadog-cli_${VERSION}_checksums.txt" | sha256sum -c -
tar -xzf "${ARCHIVE}"
install -m 0755 ddog "$HOME/.local/bin/ddog"
```

## Build locally

```bash
go build -o ddog ./cmd/ddog
./ddog --help
```

## Install from source with Go

```bash
go install github.com/nazar256/datadog-cli/cmd/ddog@latest
```

Source installs are useful for local development, but release binaries are the primary install path. Tagged release binaries include embedded version metadata; direct source installs typically report `version: dev`.

## Supported release targets

Published release archives are intended for:

- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

Linux is the main release target today.

## Where to download binaries

Download release assets from the GitHub Releases page:

- <https://github.com/nazar256/datadog-cli/releases>

## Maintainer release checklist

Before pushing a public release tag:

1. Confirm the working tree has no local secrets or generated artifacts: `git status --short --ignored`.
2. Run the basic quality gate: `go test ./...` and `go build ./cmd/ddog`.
3. Update release notes or the GitHub generated notes so users know what changed.
4. Push an intentional `vX.Y.Z` tag and let the Release workflow create a draft release.
5. Before publishing the draft, verify the archives, checksum file, and `install.sh` asset are attached.

## Shell completion

Generate completions from the installed binary:

```bash
ddog completion bash
ddog completion zsh
ddog completion fish
ddog completion powershell
```
