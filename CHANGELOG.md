# Changelog

All notable changes to AgentX are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.1] - 2026-09-11

### Added

- `ax list` readiness tree with finite states, terminal colors, authentication-gated available models, and matching JSON/YAML output.
- Session durations in the interactive recent-session selector.
- Opt-in discovery of child Agent sessions through `--include-subagents`.

### Changed

- Added distinct theme colors for every supported agent in interactive selectors, with clearer headings, metadata, and selection emphasis.
- Unified session, agent, and model selection under the same keyboard-driven interface.
- Hidden child Agent sessions by default so top-level work remains the primary discovery surface.

### Fixed

- Pi login now uses Pi's SDK OAuth provider selector and writes credentials to Pi's native authentication store.

## [0.2.0] - 2026-09-10

### Added

- Interactive, viewport-based session selection with grouped current-workspace and global sessions.
- Linux, macOS, and Windows CI coverage.
- Documentation and a recorded demo for interactive session resume workflows.

### Changed

- Faster session browsing with compact, single-line workspace context for global sessions.

## [0.1.1] - 2026-09-10

### Added

- DeepSeek Harness model discovery, credential-status reporting, and native Web UI credential setup entrypoint.
- Documentation for DeepSeek Harness profiles and native configuration ownership.

## [0.1.0] - 2026-09-10

### Added

- Native discovery, installation, authentication, model selection, and launch workflows for Claude Code, Codex CLI, Gemini CLI, OpenCode, and Pi.
- Built-in discovery and normalization of native sessions from all five agents.
- Cross-agent resume with bounded transcript handoff and no mutation of native stores.
- JSON and YAML output for AgentX-owned data and dry-run plans.
- Structured errors and opt-in raw external-call debug logging.
- Consistency audit covering CLI grammar, documentation, capabilities, and drivers.

[Unreleased]: https://github.com/ArcheMind/agentx/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/ArcheMind/agentx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/ArcheMind/agentx/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/ArcheMind/agentx/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ArcheMind/agentx/releases/tag/v0.1.0
