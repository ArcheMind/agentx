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
so. The model catalog comes from `agy models`, not a hardcoded list. Auth status is derived from the account model catalog: a nonempty successful
`agy models` response establishes a logged-in agy account. An empty catalog
reports no logged-in account; command or parsing errors leave status unknown.
No email address or subscription is inferred.

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

Final integrated checks passed as recorded below. The app/extensions owner reported
`make smoke-agy-extensions-live` passing on 2026-11-13. Evidence:
`/private/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/sm-agy-extensions-live-z5issB`.

That run exercised real `sm agent exec agy` with NDJSON stdin; the agent
recalled a randomized `SessionStart` injection without tools. Real
`session continue` cold and warm paths used new native UUIDs, preserved a
prior secret and connected MCP in the actual project workspace. Each of three
sessions fired `SessionStart` and `Stop` once. Unsupported events warned and
did not run. Native project portable hooks were not executed twice. The test
restored its owned global plugin afterward.

The integration owner recorded the following additional passes against core
binary `39104bc` on 2026-11-13. Evidence directories share the prefix
`/private/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/`:

| Check | Result | Evidence directory |
| --- | --- | --- |
| `make smoke-transcript-cross-live` | All 12 directions passed, including all six new agy directions; none skipped | `sm-transcript-cross-live-rkh5Dz` |
| `make smoke-agy-native-live` | Four strict positives passed; the unsigned native thinking observation did not retrieve its token | `sm-agy-native-live-01JV0b` |
| Strict transcript fidelity | Four providers' then-current message fixtures passed and the actual shared protocol schema validated | `agentx-transcript-fidelity-mqoUmD` |
| Nonlive cross-format conversion | All 12 directions passed | `agentx-transcript-cross-7bf9iU` |
| `make -o build smoke-agy-duplex-live` | Complete default native run passed | `sessionman-duplex-live-agy-hUjLse` |

The complete duplex run verified steer, per-RPC raw usage, MessageHost while
running, second-round reports, queued delivery, nested working directory,
depth and identity, real 60-second idle daemon exit and cold reload of the same
native ID with token recall, and termination of native descendants after Stop.
In `normal`, the native permission request was denied and no output file was
created. Native `end_turn` with no report correctly produced a failed task;
this was accepted as evidence of blocking, not a successful task result.

Earlier duplex attempts failed on native async task prompting and a macOS
symlink path assertion. Those failure records were retained. The final pass
followed harness corrections; the adapter did not gain an orchestration
fallback and the substantive assertions were not relaxed.

These observations do not establish acceptance of later source changes.
Reading a native `raw_thinking` field 16 without readable field 3 was fixed
after binary `39104bc`. The updated five-message fixture passed on frozen
source `56337f9`, with schema validation (`agentx-transcript-fidelity-JrYPiU`)
and all 12 nonlive cross directions (`agentx-transcript-cross-FfpLTr`). When both readable thinking
field 3 and raw thinking field 16 are nonempty and different, parsing fails
explicitly with a catalog warning: their mapping and signature binding are
not established, so neither value is silently discarded.

The marker fix and short coexistence probe passed against the final source:
`SM_AGY_COEXIST=1 make -o build smoke-agy-duplex-live` (exit 0), evidence
`sessionman-duplex-live-agy-wEwXwl`. The owned global generic bridge
(`sessionman_sessionman`) and temporary direct ACP namespaces returned the
correct parent/child MessageHost identities. Global-bridge child creation,
actual workspace, depth and environment binding passed. After manually
removing this test's nested sidecar state, native catalog default hiding and
`--include-subagents` visibility both passed. This proves the native
`[sm:subagent]` marker independently of sidecars; it does not establish
automatic nested-sidecar cleanup. One leftover test-owned directory was
observed. Global plugin state was restored, and the task-owned bridge pointing
to the temporary integration executable was removed before worktree cleanup.

Independent checks against frozen integration source `56337f9` passed:
`make check-update-platforms test-mcp-race test-mcp-syntax` (exit 0).
The platform target compiled Darwin, Linux and Windows for amd64 and arm64;
this is build coverage, not native Windows execution. Both the subagents and
app race suites passed, and JavaScript syntax checks included all agy smoke
scripts.

The independent acceptance owner then ran the exact default entry point on
source `1dade94`:
`make AGENTX_PROTOCOL_DIR=/Users/agent/work/agentx-protocol verify test-mcp-syntax`
(exit 0). Formatting left the tracked working tree clean. To expose the schema
result hidden by the default smoke output redirection, the owner also ran
`make -o build AGENTX_PROTOCOL_DIR=/Users/agent/work/agentx-protocol smoke-transcript-fidelity`
(exit 0), evidence `agentx-transcript-fidelity-BFD9gz`. It explicitly reported
`validated against /Users/agent/work/agentx-protocol/spec/schema/unified-transcript.schema.json`,
not skipped. The agy native output had five records, and source independence,
raw signatures, inline image and complete tool result checks were all true.
Go code and fixtures were unchanged between `56337f9` and `1dade94`; the later
change affected only the live harness.

Windows quoting and installer selection have platform unit
coverage; no real Windows execution is claimed. Native provider request effort
was not captured, so it remains unverified regardless of task success.
