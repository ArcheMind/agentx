# Installation

AgentX is a single `ax` executable. It does not install a daemon, create a configuration file, or move native agent data.

## Installer for macOS and Linux

```bash
curl -LsSf https://raw.githubusercontent.com/ArcheMind/agentx/main/install.sh | sh
```

The installer selects the current operating system and architecture, downloads the matching GitHub Release archive, verifies it against the published SHA-256 checksum, and installs `ax` to `$HOME/.local/bin`.

Choose another destination:

```bash
AX_INSTALL_DIR=/usr/local/bin \
  sh -c "$(curl -LsSf https://raw.githubusercontent.com/ArcheMind/agentx/main/install.sh)"
```

Install a specific version:

```bash
AX_VERSION=0.1.0 \
  sh -c "$(curl -LsSf https://raw.githubusercontent.com/ArcheMind/agentx/main/install.sh)"
```

The installer accepts a version with or without the `v` prefix.

## Go install

With Go 1.25 or newer:

```bash
go install github.com/ArcheMind/agentx/cmd/ax@latest
```

The binary is written to `GOBIN`, or to `GOPATH/bin` when `GOBIN` is unset.

## Prebuilt archives

Every release publishes archives for:

- macOS on Intel and Apple Silicon;
- Linux on x86-64 and ARM64;
- Windows on x86-64 and ARM64.

Download the archive and `checksums.txt` from [GitHub Releases](https://github.com/ArcheMind/agentx/releases). Verify the artifact before extracting it:

```bash
sha256sum --check checksums.txt --ignore-missing
```

On macOS, use `shasum -a 256 <archive>` and compare the result with `checksums.txt`.

## Build from source

```bash
git clone https://github.com/ArcheMind/agentx.git
cd agentx
make build
./bin/ax version
```

Use `make verify` when developing or validating a checkout.

## Native agent requirements

AgentX itself has no Node.js runtime dependency. `ax agent install`, however, delegates to the verified npm package for each supported native agent, so that command requires `npm` on `PATH`.

Already-installed agents continue to work with their own requirements. Run `ax agent list` to see what AgentX can locate.

## Uninstall

Remove the single executable from the directory where it was installed:

```bash
rm "$HOME/.local/bin/ax"
```

AgentX creates no configuration, credential, or session store to clean up.
