# Changelog

All notable changes to SessionMan are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and releases use [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.13.1] - 2026-11-13

### Fixed

- Recent sessions collapse Codex incremental rollouts into one session before
  applying display limits, matching session list and detail lookup.
- Session discovery and rollout selection are shared across listing, recent
  sessions, detail lookup, and continuation. Codex continuation selects the
  latest record by session metadata instead of file names or modification times.

## [0.13.0] - 2026-11-13

### Added

- Interactive bare `sm` checks `https://sessionman.dev/releases/latest` for a
  newer stable release and asks before installing it. Checks have a two-second
  deadline and are cached for 24 hours. Set `SM_NO_UPDATE_CHECK=1` to disable
  both checks and prompts; commands and non-interactive invocations skip them.
- Self-updates verify the platform archive's SHA-256 checksum and replace the
  resolved executable in place on macOS, Linux, and Windows. Windows uses a
  temporary helper after the original process exits. Successful updates exit
  without opening the home screen or starting an agent.

### Changed

- Recent session rows show both the first and latest user messages.
- Installation instructions use `https://sessionman.dev/install`.

### Fixed

- Dispatched subagent sessions carry a marker in their first task message so
  session listings can distinguish them from ordinary user sessions.

## [0.12.2] - 2026-11-12

### Fixed

- Imported Codex sessions record the target's effective model provider instead
  of leaving it empty and locking the resumed session. CI and release checks
  install Codex for native configuration discovery.
- Persistent native streams capture stderr synchronously with child writes,
  so completion reports do not race earlier effort telemetry on another pipe.

- Cross-agent Claude/Codex/Pi imports replay readable thinking as tagged text
  without sending foreign signatures through the target's encrypted channel.
  Opaque-only thinking is omitted and the remaining message chain is repaired;
  same-agent signature round-trips stay intact.

### Added

- `make smoke-transcript-cross-live`: extends the six conversion directions
  with real continuation and exact retrieval of source-generated unique tokens.
  Also included in `make smoke-live`; quota or credential failures fail the check.

## [0.12.0] - 2026-11-12

### Added

- One daemon per ExternalSubagent with native duplex streams: Claude stream-json,
  Codex app-server and Pi RPC. Running agents accept `delivery: "steer"`; explicit
  `"queued"` messages start a new round after the current run settles.
- `MessageHost` lets dispatched agents send progress or questions while working.
- Per-agent report/message events and `sm wait <id> --after <seq>`. One re-armed
  wait follows an agent across messages and runs.
- Pi's delegation bridge arms and re-arms event waits automatically and delivers
  wakes through `sendUserMessage`. It mirrors named delegation, delivery mode
  and the conditional host-message tool.

### Changed

- Native processes stay alive across rounds; cold continuation uses the exact
  native session. Stop kills the native process group and discards queued work.
- Codex duplex execution uses app-server without an exec fallback. Presets with
  `user_config: false` are rejected because app-server cannot honor that setting.

### Removed

- Per-task result files and their shell polling commands. Completion and mid-run
  messages use the per-agent event stream.

## [0.11.0] - 2026-11-12

### Added

- **Pi delegation bridge**: Pi gains sub-agent delegation through an embedded
  TypeScript extension that bridges all four ExternalSubagent MCP tools as
  Pi-native tools. Injected in all `ax` launch paths (interactive run, session
  resume, preset home, MCP subagent dispatch), matching Claude and Codex parity.

- **Fresh resume offloads history**: selector key `f` (and the >250K-token
  choice on Enter) resumes a session freshly — the full transcript is written
  to `~/.local/state/sessionman/history/<id>.json`, the first two and last three
  user turns are kept with tool results stubbed to the history path, and a
  pointer message is injected between them.

### Fixed

- Codex incremental rollout files (multi-agent v2) sharing the same
  `session_meta.id` are now deduplicated — only the newest rollout per session
  is kept, eliminating "session ID is ambiguous" errors on load.

- `sm session continue codex` resolves the Codex home directory via auth
  discovery, fixing resume and import when `CODEX_HOME` is not set.

- Pi session serialization now includes system messages (`SerializePi`).

- Invariant assertion added: picker selection must resolve uniquely at
  resume/export time.

## [0.10.0] - 2026-11-11

### Changed

- Background ExternalSubagent completion is delivered as a result file
  (`~/.local/state/sessionman/connections/<connection-id>/jobs/<task-id>.json`).
  Background tool responses include the path and a shell wait command, so any
  host (claude, codex, pi) can wait with a background shell instead of polling.

### Removed

- `notifications/claude/channel` delivery, the `experimental/claude/channel`
  capability, `SM_CLAUDE_CHANNELS` flag injection, and the
  `smoke-mcp-channel` / `smoke-mcp-host-live` / `verify-mcp-host-evidence`
  targets (replaced by `smoke-mcp-jobfile`).

## [0.9.2] - 2026-11-10

### Fixed

- Pi model list and `sm auth list` no longer depend on locating Pi's Node SDK
  beside the `pi` executable, which failed under Pi's managed install
  (`curl pi.dev/install.sh`). Both now read `pi --list-models`, which Pi
  already restricts to providers with configured auth, so custom
  `models.json` providers and env-var credentials are listed.

## [0.9.1] - 2026-11-10

### Changed

- Replace runtime fallback with idempotent install-time migration: first run
  renames `~/.sessionman/profiles/` → `presets/` and `PROFILE.md` → `PRESET.md`.

## [0.9.0] - 2026-11-10

### Changed

- **Vocabulary rename: profile → preset.** CLI command is now `sm preset <list|show>`;
  configuration directory is `~/.sessionman/presets/<name>/PRESET.md`; MCP catalog
  text uses "preset" throughout. Go package `internal/profiles` → `internal/presets`;
  struct `Profile` → `Preset`. Smoke test files and Makefile targets renamed accordingly.

## [0.8.5] - 2026-11-07

### Fixed

- `sm session continue codex` imports history and reads the result from the
  same Codex home as the native turn.

## [0.8.4] - 2026-11-05

### Fixed

- `sm session continue`: unsigned thinking (synthetic or cross-agent, with no
  provider signature) is no longer silently dropped from the model's context.
  Codex, Claude and Pi all ignore unsigned thinking in imported history, so it
  now serializes as its own `<think>`-wrapped text block in its original
  position for all three agents; signed thinking keeps the native channel, so
  same-agent round-trips stay lossless. Every provider import lifts a text block
  wholly wrapped in a single `<think>` pair back into a thinking block, so the
  unified transcript round-trips.

## [0.8.3] - 2026-11-04

### Fixed

- `sm session continue` to codex: message ids the OpenAI Responses API rejects
  (not `msg`-prefixed or longer than 64 characters, e.g. namespaced continue
  deltas "<runId>:msg_...") are mapped on import to "msg_" + 60 hex of their
  SHA-256, with parentId mapped the same way so the chain stays intact.
  Previously, once such a delta was in a transcript, every later codex turn on
  that chain failed with 400 `string_above_max_length`. Like tool id
  sanitization, this lives on the continue import path only; serializers keep
  ids verbatim.

## [0.8.2] - 2026-11-04

### Fixed

- Session commands and the interactive recent-session picker no longer fail with
  `bufio.Scanner: token too long` when a native JSONL record exceeds 8 MiB (for
  example, a large tool output). Full records are preserved instead of truncated
  or skipped.

## [0.8.1] - 2026-10-31

### Fixed

- `sm session continue`: tool ids the native APIs reject — the pre-refactor
  harness's scoped ids ("native-<uuid>:<raw>") and composite ids
  ("call_x|fc_y") — are mapped on import to deterministic API-safe ids
  (charset [A-Za-z0-9_-], at most 64 characters), keeping tool_use/tool_result
  pairs intact; already-safe native ids pass untouched. Previously any
  transcript carrying such tool history failed every continue: Anthropic 400
  (tool_use.id charset) and OpenAI Responses 400 (call_id length). Like the
  positional rechain, this lives on the continue import path only; serializers
  keep ids verbatim for export/import fidelity. The live regression
  (`make smoke-continue-live`) now also runs a real tool turn, rewrites its ids
  into the legacy scoped shape, and continues over them.

## [0.8.0] - 2026-10-31

### Added

- `sm session continue <claude|codex|pi>`: headless one-turn transcript continuation.
  The imported history's parent chain is rewritten positionally: Claude and Pi
  reconstruct a session as the newest record's ancestor chain, so the dangling
  parentIds normal in delta-appended transcripts would otherwise orphan the
  history (the second turn silently lost the first turn's context). Export and
  import fidelity are untouched; serializers keep parentId verbatim. Live
  regression: `make smoke-continue-live` (SM_CONTINUE_AGENTS=claude[,codex,pi]). Reads a unified transcript (file or stdin) ending in a user message, imports the history into the native store, runs one turn with the trailing user message as the prompt, and prints the newly recorded messages as a transcript delta on stdout. Flags: `--model`, `--effort` (codex reasoning effort / claude effort / pi thinking), `--append-system-prompt`, `--mcp-config <file|json>` (unified `mcpServers` map translated to `--mcp-config --strict-mcp-config` for claude and `-c mcp_servers.*` for codex; pi is rejected, it has no MCP mechanism), `--mode <normal|yolo>` (normal passes no permission or sandbox flags; yolo bypasses them, rejected for pi), `--workspace`. Delta message ids are namespaced per call so repeated turns append cleanly to one caller-owned transcript.

### Changed

- ExternalSubagent permissions collapse to one `mode` parameter: `normal` (default) passes no permission or sandbox flags, so each agent's own configuration decides (Codex keeps `approval_policy="never"` because headless runs cannot answer prompts); `yolo` bypasses them (Claude `--dangerously-skip-permissions`, Codex `sandbox_mode="danger-full-access"`; rejected for Pi). Reports show `Mode:` instead of `Write scope:`.
- `isolation: "worktree"` only chooses where the agent works and no longer binds a sandbox to the worktree.

### Removed

- ExternalSubagent `write` and `writable_roots` parameters, the deprecated native `mode` values (`acceptEdits`, `auto`, `bypassPermissions`, `default`, `dontAsk`, `plan`), and the SessionMan-injected Codex `read-only`/`workspace-write` sandbox with its network and tmpdir restrictions. No compatibility aliases.
- `make smoke-mcp-policy-live`, which tested the removed write scope.
- Preset `sandbox` field and its `ax` preset editor entry: it only tuned the removed write-mode Codex sandbox. Presets that still set `sandbox` are rejected as having an unknown field; delete the line.

## [0.7.0] - 2026-10-25

### Added

- Lossless transcript round-trip: adopt agentx-protocol/go v0.4.0 with image blocks, structured tool results, system role, and tool result materialization rules.
- `recordedAt` field preserves Pi's record-level write timestamp (distinct from message event time) across round-trips.
- Codex `input_image` ↔ unified `image` block bidirectional mapping.
- Pi `system` role parsing and serialization (native `sections` → text, text → `content`).
- Offline transcript fidelity smoke tests and cross-provider diagonal smoke tests (claude↔codex).
- `sm convert` command for stdin-based transcript format conversion.

### Changed

- Unified transcript field `agent` renamed to `provider` (agentx-protocol/go v0.3.0+).
- Pi/gemini/opencode import now uses fresh UUID filenames (prevents silent overwrite on re-import).

## [0.6.2] - 2026-10-25

### Changed

- `sm claude` attaches the SessionMan MCP server and ExternalSubagent tools unconditionally, like `sm codex`; `sm help` examples now show interactive hand-off and calling a Claude Code subagent from Codex.

### Removed

- The Claude MCP registration check and the `sm <agent> --subagents` flag.

## [0.6.1] - 2026-10-25

### Changed

- `sm help` lists each command once and ends with examples for the three main uses: one list of every agent's work, continuing a session in another agent, and calling other agents' presets from Claude.

## [0.6.0] - 2026-10-25

### Changed

- `sm preset serve` is now `sm mcp serve`; `sm mcp` gains `serve` beside `list` and `show`.
- `preset` is now `preset`: `sm preset list|show`, `~/.sessionman/presets/<name>/PRESET.md`, and the matching interactive home, MCP instructions, and tool descriptions.
- `provider` is now `agent` in `--source <agent>`, the `session list` output and `--sort agent`, the exported filename `<agent>-<title>-<id8>.json`, and the hook `__dispatch --agent` flag.
- The unified transcript field `provider` is now `agent` (agentx-protocol `go/v0.2.0`, module `github.com/ArcheMind/agentx-protocol/go`); transcripts exported by earlier versions no longer import.
- `sm list` is now `sm status`.

### Removed

- `sm convert`.

## [0.5.0] - 2026-10-23

### Added

- `sm session export <session-id> [--source <provider>]` streams a session as a unified transcript.
- `sm session import <target-agent> [file]` writes a transcript into the target agent's native store and prints the new session id, without launching the agent.
- `sm --resume <transcript-file>` imports a transcript and resumes it interactively in one step.
- `e` in the interactive home exports the selected session to `<provider>-<title>-<id8>.json` in the current directory.

## [0.4.2] - 2026-10-23

### Added

- Presets for every supported agent, including Claude: `sm claude` and other non-Codex/Pi presets now run as ExternalSubagents through their own native driver.
- `sandbox`, `user_config`, and `flags` preset fields, shown and edited in the interactive preset editor. `sandbox` and `user_config` translate to native flags; `flags` appends raw native arguments.
- SessionMan MCP injection for interactive Codex launches and for dispatched Codex ExternalSubagents, so nested delegation works without extra setup.

### Changed

- Security is expressed through preset fields and enforced by the native runtime. SessionMan no longer hardcodes per-agent sandbox, approval, or permission reasoning.
- Preset `Limits` and the tool descriptions describe only SessionMan's own parameters: write, isolation, writable_roots, sandbox, user_config, flags, and effort.
- Effort and model evidence mismatches are reported as warnings instead of failing the turn.
- `sm claude` injects `--allowedTools` with exactly the four SessionMan tools and no longer adds `--dangerously-load-development-channels`; background delivery is the host's own opt-in.
- Every preset appears in the MCP catalog; the `subagent` gate and the `general-purpose` filter are gone.

### Removed

- Declared roles: `~/.sessionman/roles/`, `ROLE.md`, and the `subagent_type` tool parameter.

## [0.4.1] - 2026-10-21

### Fixed

- Subagent catalog readiness tests stub both `codex` and `pi`, so they no longer depend on a locally installed `pi`. `v0.4.0` was tagged on a commit where this test failed on Linux, so its release workflow stopped before publishing; `v0.4.1` is the first published release carrying the `0.4.0` changes.

## [0.4.0] - 2026-10-21

### Added

- ExternalSubagent MCP tools (`ExternalSubagent`, `SendExternalSubagentMessage`, `StopExternalSubagent`, `listExternalSubagentSessions`) served by `sm preset serve`. Codex and Pi execute the subagent natively, with no relay inference call.
- A single preset collection at `~/.sessionman/presets/` shared by the bare `ax` home screen and the MCP catalog, with create, edit, rename, duplicate, and delete flows, plus `sm preset list` and `sm preset show`.
- Per-turn effort verification against native evidence for Codex and Pi; a mismatch fails the turn instead of being silently accepted.
- Call-time write scope: `write` with `writable_roots`, or `isolation: "worktree"` to run the subagent in a new git worktree that is the only writable root.
- `sm skill list|show` and `sm mcp list|show` inventory by filesystem source, scope, applicable readers, and known projections.
- Resume now converts a session to the target agent's native transcript format and loads it natively instead of sending a text summary.
- Home screen presets rendered as single-line rows alongside recent sessions.

### Changed

- MCP tools and reports are named around ExternalSubagent sessions.
- Registration with Claude is delegated to `claude mcp`; the preset collection is the only SessionMan configuration.
- The tool metadata index is bounded, and reports state when entries are omitted.
- Writable roots are resolved by directory identity, symlinks included, and the working directory stays always writable in non-isolated write mode as documented native Codex behavior.
- Interactive selectors keep a stable layout without redraw drift, with a narrower preset combination column.

### Removed

- `~/.config/sessionman/subagents.json` (its `enabled` switch, `default`/`options`, and `grants`) is no longer read; call-time write scope replaces bounded-write grants and approval had no replacement.
- The `enable` and `disable` subcommands.

## [0.3.1] - 2026-09-30

### Added

- Portable hooks using the Claude/Codex `hooks.json` structure at `~/.agents/hooks.json` and `.agents/hooks.json`, with normalized `SessionStart`, `SessionEnd`, `UserPromptSubmit`, and `Stop` events, `sm hook list`, and per-agent bridge projection.
- Unified skill discovery and installation across agents.
- Native hooks and skills smoke coverage.

### Fixed

- Codex entitled models are discovered natively.
- Windows smoke command updated for the current CLI surface.

## [0.3.0] - 2026-09-20

### Added

- `sm convert to|from <provider>`: reverse serializers from the unified transcript to native formats for Claude, Codex, Pi, and Gemini.
- Structured transcript content blocks replacing flat message content.

### Changed

- The unified transcript format is owned by `agentx-protocol`; SessionMan aliases its types and defines no message shapes of its own.
- CLI verb patterns aligned across resource branches.

## [0.2.2] - 2026-09-12

### Fixed

- Pi custom API provider discovery is narrowed to user-declared providers before models are listed.
- Interactive resume demo flow restored.

## [0.2.1] - 2026-09-11

### Added

- `sm list` readiness tree with finite states, terminal colors, authentication-gated available models, and matching JSON/YAML output.
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
- Documentation for DeepSeek Harness presets and native configuration ownership.

## [0.1.0] - 2026-09-10

### Added

- Native discovery, installation, authentication, model selection, and launch workflows for Claude Code, Codex CLI, Gemini CLI, OpenCode, and Pi.
- Built-in discovery and normalization of native sessions from all five agents.
- Cross-agent resume with bounded transcript handoff and no mutation of native stores.
- JSON and YAML output for SessionMan-owned data and dry-run plans.
- Structured errors and opt-in raw external-call debug logging.
- Consistency audit covering CLI grammar, documentation, capabilities, and drivers.

[Unreleased]: https://github.com/ArcheMind/agentx/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/ArcheMind/agentx/compare/v0.4.2...v0.5.0
[0.4.2]: https://github.com/ArcheMind/agentx/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/ArcheMind/agentx/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/ArcheMind/agentx/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/ArcheMind/agentx/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/ArcheMind/agentx/compare/v0.2.2...v0.3.0
[0.2.2]: https://github.com/ArcheMind/agentx/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/ArcheMind/agentx/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/ArcheMind/agentx/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/ArcheMind/agentx/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ArcheMind/agentx/releases/tag/v0.1.0
