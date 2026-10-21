# Changelog

All notable changes to AgentX are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.1] - 2026-10-21

### Fixed

- Subagent catalog readiness tests stub both `codex` and `pi`, so they no longer depend on a locally installed `pi`. `v0.4.0` was tagged on a commit where this test failed on Linux, so its release workflow stopped before publishing; `v0.4.1` is the first published release carrying the `0.4.0` changes.

## [0.4.0] - 2026-10-21

### Added

- ExternalSubagent MCP tools (`ExternalSubagent`, `SendExternalSubagentMessage`, `StopExternalSubagent`, `listExternalSubagentSessions`) served by `ax preset serve`. Codex and Pi execute the subagent natively, with no relay inference call.
- A single preset collection at `~/.agentx/presets/` shared by the bare `ax` home screen and the MCP catalog, with create, edit, rename, duplicate, and delete flows, plus `ax preset list` and `ax preset show`.
- Declared roles at `~/.agentx/roles/` (`ROLE.md`) selected by `subagent_type`, carried by each runtime natively and verified per turn.
- Per-turn effort verification against native evidence for Codex and Pi; a mismatch fails the turn instead of being silently accepted.
- Call-time write scope: `write` with `writable_roots`, or `isolation: "worktree"` to run the subagent in a new git worktree that is the only writable root.
- `ax skill list|show` and `ax mcp list|show` inventory by filesystem source, scope, applicable readers, and known projections.
- Resume now converts a session to the target agent's native transcript format and loads it natively instead of sending a text summary.
- Home screen presets rendered as single-line rows alongside recent sessions.

### Changed

- MCP tools and reports are named around ExternalSubagent sessions.
- Registration with Claude is delegated to `claude mcp`; the preset and role collections are the only AgentX configuration.
- The tool metadata index is bounded, and reports state when entries are omitted.
- Writable roots are resolved by directory identity, symlinks included, and the working directory stays always writable in non-isolated write mode as documented native Codex behavior.
- Interactive selectors keep a stable layout without redraw drift, with a narrower preset combination column.

### Removed

- `~/.config/agentx/subagents.json` (its `enabled` switch, `default`/`options`, and `grants`) is no longer read; call-time write scope replaces bounded-write grants and approval had no replacement.
- The `enable` and `disable` subcommands.

## [0.3.1] - 2026-09-30

### Added

- Portable hooks using the Claude/Codex `hooks.json` structure at `~/.agents/hooks.json` and `.agents/hooks.json`, with normalized `SessionStart`, `SessionEnd`, `UserPromptSubmit`, and `Stop` events, `ax hook list`, and per-agent bridge projection.
- Unified skill discovery and installation across agents.
- Native hooks and skills smoke coverage.

### Fixed

- Codex entitled models are discovered natively.
- Windows smoke command updated for the current CLI surface.

## [0.3.0] - 2026-09-20

### Added

- `ax convert to|from <provider>`: reverse serializers from the unified transcript to native formats for Claude, Codex, Pi, and Gemini.
- Structured transcript content blocks replacing flat message content.

### Changed

- The unified transcript format is owned by `agentx-protocol`; AgentX aliases its types and defines no message shapes of its own.
- CLI verb patterns aligned across resource branches.

## [0.2.2] - 2026-09-12

### Fixed

- Pi custom API provider discovery is narrowed to user-declared providers before models are listed.
- Interactive resume demo flow restored.

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

[Unreleased]: https://github.com/ArcheMind/agentx/compare/v0.4.1...HEAD
[0.4.1]: https://github.com/ArcheMind/agentx/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/ArcheMind/agentx/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/ArcheMind/agentx/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/ArcheMind/agentx/compare/v0.2.2...v0.3.0
[0.2.2]: https://github.com/ArcheMind/agentx/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/ArcheMind/agentx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/ArcheMind/agentx/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/ArcheMind/agentx/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ArcheMind/agentx/releases/tag/v0.1.0
