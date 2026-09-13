# Verified Repository Productization and Release State

## Status

The GitHub repository productization requirement is complete and published. The latest verified release is `v0.2.2`, pointing to commit `5f54619`; the four Pi provider-discovery fix and documentation commits have been pushed to `origin/main`.

## Product entry point

**Conclusion:** The repository now provides a complete open-source product entry point: product positioning and narrative, installation and first-run guidance, architecture and product-principles documentation, troubleshooting, changelog, contribution and governance paths, and issue and pull-request templates. The installer, public Go module path, and GoReleaser configuration are established. The GitHub description and topics are set.

**Why:** A prospective user can discover, understand, trust, install, try, and contribute to `agentx` from the repository without first understanding its implementation history.

**Files:** `README.md`, `install.sh`, `go.mod`, `.goreleaser.yml`, `docs/`, `CHANGELOG.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/`, `.github/PULL_REQUEST_TEMPLATE.md`

## Release and automation

**Current release:** GitHub release `v0.2.2` is public, neither a draft nor a prerelease. Release workflow run `34728994409` completed successfully. All six archives for macOS, Linux, and Windows on `amd64` and `arm64`, plus `checksums.txt`, are uploaded. This patch ships the Pi custom API provider discovery and authentication-source filtering fix in `internal/drivers/pi_sdk_providers.mjs`, preserving the second model-filtering pass.

**Conclusion:** GitHub release `v0.2.0` was published on 2026-09-10 America/Los_Angeles (`publishedAt` 2026-09-11T01:14:31Z). It is neither a draft nor a prerelease. Release workflow run `34549808674` succeeded after local `make verify` passed. The release contains six archives covering Darwin, Linux, and Windows on both `amd64` and `arm64`, plus `checksums.txt`. Dependabot is enabled; its first pull requests upgrading `actions/checkout`, `actions/setup-go`, and GoReleaser actions all passed checks and were squash-merged.

**Why:** Installation artifacts, repeatable releases, continuous verification, security analysis, and dependency-maintenance automation are live rather than documented aspirations.

**Files:** `.github/workflows/`, `.github/dependabot.yml`, `.goreleaser.yml`

## Windows installation and Agent management validation

**Conclusion:** Windows has release configuration and downloadable ZIP artifacts, but `install.sh` explicitly rejects Windows, so no Windows shell installer is provided. CI defines Linux, macOS, and Windows coverage. `verify-windows` runs on `windows-latest` with PowerShell, executes `go vet`, `go test`, and the dedicated audit tests, then builds and runs `ax.exe`; it covers `version`, JSON/YAML `agent list`, dry-run `agent install`, auth, and run operations, plus `session providers`. `verify-macos` runs `make verify` on `macos-latest` and confirms that it leaves no generated diff. These jobs are pushed on `main`; the `v0.2.0` release workflow also successfully produced native archives for all three operating systems and both supported architectures. The Agent-management implementation (`list`, `which`, and `install`) uses Go command/path primitives and has no identified POSIX-only code path.

**Why:** Native CI jobs provide repeatable cross-platform verification, while the successful release workflow establishes that the configured Darwin, Linux, and Windows artifacts can be produced for the supported architectures. Installer support remains distinct from artifact availability.

**Files:** `install.sh`, `.goreleaser.yml`, `internal/drivers/package.go`, `internal/drivers/registry.go`, `.github/workflows/ci.yml`, `README.md`, `Makefile`

## GitHub settings

- `main` branch protection is active; the user manually completed the repository setting.
- Whether GitHub private vulnerability reporting is enabled has not been verified.

Private vulnerability reporting is an external repository setting, not a missing repository file. Do not represent it as complete without fresh verification.
