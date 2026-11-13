# Installation

SessionMan is a single `sm` executable. It does not install a daemon, create a configuration file, or move native agent data.

## Installer for macOS and Linux

```bash
curl -fsSL https://sessionman.dev/install | sh
```

The installer selects the current operating system and architecture, downloads the matching GitHub Release archive, verifies it against the published SHA-256 checksum, and installs `sm` to `$HOME/.local/bin`.

Choose another destination:

```bash
SM_INSTALL_DIR=/usr/local/bin \
  sh -c "$(curl -fsSL https://sessionman.dev/install)"
```

Install a specific version:

```bash
SM_VERSION=0.1.0 \
  sh -c "$(curl -fsSL https://sessionman.dev/install)"
```

The installer accepts a version with or without the `v` prefix.

## Startup updates

Only bare `sm` with both stdin and stdout attached to a terminal checks for updates. The check starts at `https://sessionman.dev/releases/latest`, follows redirects, and has a two-second total network deadline. Network failures do not prevent the home screen from opening. Attempts are cached for 24 hours under the operating system's user cache directory in `sessionman/update-check.json`.

When a newer stable release includes an archive for the current OS and architecture plus `checksums.txt`, SessionMan asks `Update now? [y/N]`. Entering `y` or `yes` downloads and verifies the archive's SHA-256 checksum before replacing the current executable. Declining opens the home screen. A successful update exits without opening the home screen or starting an agent. Installation failures report an error and exit; SessionMan does not request elevated privileges.

macOS and Linux replace the executable in its existing directory. Windows starts a temporary helper that waits for the original process to exit, replaces the executable, and then exits. Symlink installations update the resolved executable. The shell installer is not used for self-updates.

Set `SM_NO_UPDATE_CHECK=1` to disable both checking and prompting, including cached prompts. For example, use `SM_NO_UPDATE_CHECK=1 sm` on macOS/Linux, or `$env:SM_NO_UPDATE_CHECK='1'; sm` in PowerShell. Agent commands, resume commands, MCP, all other subcommands, and invocations with any arguments skip the check. Development and prerelease version strings skip it as well.

`SM_LOG=debug` records the request URL, final response headers, raw response bytes as base64, and original errors. The lookup endpoint currently redirects to GitHub; until the server changes that response, lookup and artifact downloads still require GitHub access.

The endpoint's JSON contract is `tag_name`, `draft`, `prerelease`, and `assets` entries containing `name` and `browser_download_url`. A future self-hosted response can preserve these fields and point artifact URLs at another origin without changing the client.

## Go install

With Go 1.25 or newer:

```bash
go install github.com/ArcheMind/agentx/cmd/sm@latest
```

The binary is written to `GOBIN`, or to `GOPATH/bin` when `GOBIN` is unset.

## Prebuilt archives

Every release publishes archives for:

- macOS on Intel and Apple Silicon;
- Linux on x86-64 and ARM64;
- Windows on x86-64 and ARM64.

Download the archive and `checksums.txt` from [GitHub Releases](https://github.com/ArcheMind/sessionMan/releases). Verify the artifact before extracting it:

```bash
sha256sum --check checksums.txt --ignore-missing
```

On macOS, use `shasum -a 256 <archive>` and compare the result with `checksums.txt`.

## Build from source

```bash
git clone https://github.com/ArcheMind/sessionMan.git
cd sessionMan
make build
./bin/sm version
```

Use `make verify` when developing or validating a checkout.

## Native agent requirements

SessionMan itself has no Node.js runtime dependency. `sm agent install`, however, delegates to the verified npm package for each supported native agent, so that command requires `npm` on `PATH`.

Already-installed agents continue to work with their own requirements. Run `sm agent list` to see what SessionMan can locate.

## Uninstall

Remove the single executable from the directory where it was installed:

```bash
rm "$HOME/.local/bin/sm"
```

SessionMan creates no configuration, credential, or session store to clean up.
