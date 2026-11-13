# Antigravity CLI acceptance

The native integration targets Antigravity CLI 1.5.8. This document separates
repeatable checks from actual native observations; a listed command is not a
claim that it passed. Final evidence will record the exact command, result,
and retained evidence directory.

## Acceptance matrix

| Surface | Required observation | Repeatable check |
| --- | --- | --- |
| Lifecycle | Detection, installation plan, native login/logout, model catalog, launch, presets, CLI/TUI help | `make verify check-update-platforms` |
| Native sessions | New UUID and summary entry; source read-only; SQLite/WAL; list/show/preview/trim/import/export/continue; standalone images and complete linked tool results | `make verify smoke-transcript-fidelity` |
| Cross-format conversion | Every ordered pair of Claude, Codex, Pi and agy; lawful thinking/signature degradation; native destination validation | `make smoke-transcript-cross` |
| Cross-format inference | All 12 ordered pairs retrieve a source-only unique marker after native resume; no skipped direction | `make smoke-transcript-cross-live` |
| Native agy replay | Body, foreign thinking projection, complete disconnected MCP result, inline image; unsigned native thought recorded as an observation | `make smoke-agy-native-live` |
| ACP duplex | Warm process, steer, queue, multiple in-flight results, raw usage per RPC, stop reasons, exact cold session, MessageHost, nested MCP, permissions and cancellation | `make smoke-agy-duplex-live` and deterministic/race checks |
| Shared extensions | agy skills, hooks, owned plugin bridge and inventory preserve user configuration and expose accurate native semantics | Owner live acceptance and `make verify` |

## Protocol boundaries

Ordinary `sm agy` launches the native CLI. ExternalSubagents use `agy acp
--stdio` with ACP protocol version 1. ACP initializes the native session,
sets the exact model and requested `thought_level`, and keeps the process
alive between messages. Cold continuation loads the original session ID;
a different returned ID fails explicitly.

Single-prompt usage retains the native `usage` object. Overlapping prompt
results retain each RPC ID, `stopReason` and raw usage under `prompts`; these
are not summed or normalized. `max_tokens`, `max_turn_requests`, `refusal`,
invalid stop reasons and native RPC errors are not silently retried.

`normal` passes no bypass flag and preserves native permission configuration.
The headless ACP bridge cancels interactive permission requests it cannot
present. `yolo` passes the native `--dangerously-skip-permissions` flag.
Neither mode inherits the host agent's authorization.

A successful `thought_level` configuration response establishes acceptance
of the request, not the effort sent to the provider. The provider request is
not recorded by this adapter; agy effort remains unverified and reports say
so. The model catalog comes from `agy models`, not a hardcoded list. No
native authoritative account-status query is available, so auth status is
unsupported; login or model availability is not inferred from credentials.

Claude stream-json, Codex app-server and Pi RPC remain native. Migrating
those adapters to ACP is separate work.

## Hook semantics

agy 1.5.8 exposes `SessionStart`, `PreToolUse`, `PostToolUse`,
`PreInvocation`, `PostInvocation` and `Stop`. It does not expose
`SessionEnd` or `UserPromptSubmit`. `PreInvocation` fires for each model
invocation and must not be presented as a user prompt submission event.
`Stop` ends an execution loop, not the conversation session or CLI process.
Native hook output can inject steps or continue/terminate the loop; a portable
hook bridge must document the narrower behavior it actually supports.

## Evidence

Final integrated acceptance is pending. No live pass is claimed here yet.
