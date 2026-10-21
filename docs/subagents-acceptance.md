# MCP subagent acceptance evidence

Recorded 2026-10-20. These checks concern the new single-call MCP contract,
not the removed discover/define/native-Agent bridge. Deterministic tests verify
contract/error/concurrency behavior. Live tests separately establish native
engine operation and actual Claude notification consumption. Fixture passes are
never counted as real inference requests.

## Current ExternalSubagent naming

The current tools are `ExternalSubagent`, `SendExternalSubagentMessage`,
`StopExternalSubagent`, and `listExternalSubagentSessions`. They replace the
earlier `Agent`, `SendMessage`, `TaskStop`, and `ListAgents` MCP names without
aliases. Parameter keys, configured combination names, permissions and lifecycle
semantics remain unchanged. The server identity and channel source remain AgentX
and `agentx`; native Claude tools are not renamed.

The live observations and quoted tool names below are historical pre-rename
records. They establish lifecycle behavior before this naming change, not a new
live run of the renamed tools or natural-language discovery. Current mechanical
coverage verifies the four names/schema/dispatch, old-name rejection, unchanged
parameter/model selection, and lifecycle regressions without additional model
inference. Current smoke harnesses use new names; saved old transcripts require
explicit `MCP_EVIDENCE_FORMAT=historical` when running
`make verify-mcp-host-evidence`.

Rename verification passed via `make verify test-mcp-race test-mcp-syntax
smoke-mcp-management smoke-mcp-contract`. The stdio binary check exposed exactly
the four new names, rejected all four old names, preserved the ten creation
parameter keys/server identity/protocol, and listed the empty current connection
without creating any work. It used no model inference. Existing lifecycle tests
now call the renamed tools; historical host evidence is read only in explicit
historical mode.

## Execution policy, approval and authentication (backlog 004 G01–G03)

Recorded 2026-10-21 with Codex CLI 0.163.0 (`gpt-5.6-luna`, `low`) and Pi CLI
0.84.4 (`openai-codex/gpt-5.6-luna`). The runs are real native execution
through the built AgentX stdio MCP server, not through a Claude host. Each Codex
probe is one command in its own ExternalSubagent, so success does not depend on a
model copying several commands correctly. Agent reports are not evidence.

Codex 0.163 code mode runs shell commands and `apply_patch` inside its `exec`
tool. Those nested calls do not appear in the `codex exec --json` event stream.
For each probe, the smoke reads the native thread ID from the AgentX debug log
(`thread.started`). It then opens `$CODEX_HOME/sessions/**/rollout-*-<thread>.jsonl`
and asserts two things from `custom_tool_call` and `custom_tool_call_output`:
the exact command or patch on the target path was actually attempted, and the
runtime returned the expected result. It finally checks the target's filesystem
state. Pi has no rollout of this kind, so its check is filesystem state plus an
assertion that its session file contains no `write`/`edit`/`bash` tool call.
Two consecutive per-probe `make smoke-mcp-policy-live` runs passed. Evidence
directories (each with `server-debug.log`; rollout paths are listed in the
smoke output): `/private/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-mcp-policy-x6i3XY`
and `…/agentx-mcp-policy-Tjvj8i`. An earlier version put six commands in one
agent and judged only by file absence. During independent acceptance, that
version failed once in two runs: the model miscopied a path and invented
results. It is superseded.

| ID | Status | Observation |
| --- | --- | --- |
| A01 | Pass (Codex); Pi write gap open | Codex `bounded-write` (roots `root`, `extra`): rollouts show both `printf` writes executed with exit 0, and `root/in.txt` / `extra/extra.txt` contain the requested content. Codex `read-only` in the same directory: the rollout shows the write executed and failing with `operation not permitted`, and `root/ro.txt` is absent. Pi `read-only` left `root/pi.txt` absent; Pi reported no write tool. Every report carried its actual `Grant:` line. Pi `bounded-write` is rejected at configuration time. |
| A02 | Pass (Codex) | Shell writes, OS layer: in each rollout the write was executed and failed with `operation not permitted`. Targets: new `outside/out.txt` (absent afterwards), overwrite of `outside/existing.txt` (still `original`), the `/tmp` probe (absent), and the explicitly denied target `root/.git/agentx-denied.txt` (Codex-protected inside a writable root; absent). Patch, application layer: `apply_patch` adding `outside/patched.txt` was rejected by Codex's patch approval check (`patch rejected: writing outside of the project; rejected by user approval settings`) before any write; the file is absent. This rejection is not the OS sandbox. User-configurable deny subpaths are not offered because no runtime enforcement was established. |
| A03 | Partial | Covered: the no-approver scenario (`approval:"never"`), observed above with the denial returned to the agent in both layers. Out-of-scope shell writes fail in the OS sandbox (`operation not permitted`). Out-of-scope `apply_patch` is rejected by Codex's application-level patch approval with no one asked (`rejected by user approval settings`). Pending: approve and deny scenarios. Non-interactive Codex/Pi execution has no approval route to a human, and AgentX does not forward runtime approval requests to Claude. Non-`never` values are rejected rather than claimed. Pi's boundary is described everywhere as a tool allowlist, not an OS sandbox. |
| A04 | Pass | Configured sources: `CODEX_HOME=~/.codex` (chatgpt) and `PI_CODING_AGENT_DIR=~/.pi/agent` (oauth), visible in reports, catalog and `status`. An empty `CODEX_HOME` failed with `Not logged in; no fallback identity`. A method mismatch (configured `api-key`, actual ChatGPT) failed. An empty Pi dir failed with `credentials_not_configured; no fallback identity`. None of these started native execution (debug log). Twelve inherited credential-like variables were removed. Seven credential values (from `OPENAI_API_KEY` and both `auth.json` files) appeared in neither debug logs nor reports. Direct check: `PI_CODING_AGENT_DIR=<empty> pi auth check --model openai/gpt-5` is `ready` via the inherited `OPENAI_API_KEY`, and `not_ready` once it is removed. This is the implicit fallback AgentX now blocks. Identity here means source directory plus login method; the account inside a source is not verified, so switching accounts within the same `CODEX_HOME` is not detected. |

What each runtime actually enforces:

| Restriction | Codex | Pi |
| --- | --- | --- |
| File writes by commands | Native OS sandbox (`read-only` / `workspace-write` with roots) | Tool allowlist `read,grep,find,ls` only; application-level, not OS |
| File writes by `apply_patch` | Codex application-level patch approval rejects paths outside the roots | No edit/write tool enabled |
| Network from commands | Sandbox blocks | No shell tool is enabled; not OS-enforced |
| Reads | Not path-restricted | Not path-restricted |
| User MCP servers/hooks/notify | Not loaded (`--ignore-user-config --ignore-rules --disable hooks`); before this change user MCP servers such as a node REPL ran outside the sandbox | Extensions, skills, prompt templates and project approval disabled |
| Approval | `approval_policy="never"`; no human path | None exists |
| Auth source | `CODEX_HOME` + `codex login status` preflight (method only, not account) | `PI_CODING_AGENT_DIR` + `pi auth check` preflight (provider and auth type, not account) |

Deterministic coverage (`go test ./internal/subagents ./internal/runtime`) adds
the following:
- Grant validation rejects missing or orphan grants, unknown or empty policies,
  non-`never` approval, bad roots, Pi `bounded-write`, and wrong auth sources or
  methods.
- Policy maps to the expected Codex flags.
- A working directory outside the roots is rejected before any work starts.
- Auth failures never reach native execution.
- A grant change rejects continuation.
- Debug logs redact credentials, and unset variables do not reach the child.

Note (2026-10-21): the user changed the grant/preset mismatch semantics. A
callable preset without a grant is now only not callable, and an orphan grant or
a grant on an interactive-launch-only preset is reported as a problem without
effect. Structural grant errors still reject the whole configuration. This takes
effect from integrate/wave-1 commit 0b7dbf3 ("Reject only the affected preset
when grants and presets disagree"). The missing/orphan-grant rejection verified
above reflects the earlier semantics.

## Isolation (backlog 004 G05–G06)

Recorded 2026-10-21 with Codex CLI 0.163.0 (`gpt-5.6-luna`, `low`), Pi CLI
0.84.4 (`openai-codex/gpt-5.6-luna`) and git 2.39.5, through the built AgentX
stdio MCP server (`make smoke-mcp-isolation-live`), not through a Claude host.
The original checkout started dirty: a modified tracked file (`sub/tracked.txt`)
and an untracked file (`draft.txt`). The smoke snapshots `git status
--porcelain --untracked-files=all` plus every file's content outside `.git`
before and after all runs. Codex sandbox evidence comes from rollouts, as in the
G01–G03 section. Evidence directory (with `server-debug.log` and
`smoke-output.json`):
`/private/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-mcp-isolation-sNefQJ`.

| ID | Status | Observation |
| --- | --- | --- |
| A06 | Pass (Codex, Pi read-only) | Success: bounded-write grant with root = the repository. The native plan ran in `<worktree>/sub` with `writable_roots=["<worktree>"]` and without the repository path. Rollout `…/rollout-2026-10-21T05-10-57-01a15e5f-2a8d-7350-8b22-eda5b9b39f0b.jsonl` shows `printf isolated > agent.txt` with exit 0. The file exists only in the worktree, and the report names path, branch `agentx/<id>`, base commit and `?? sub/agent.txt`. Escape: an absolute write to the original `repo/sub/escape.txt` failed with `operation not permitted` (rollout `…-01a15e5f-2a8e-7902-a74b-7305306ed11a.jsonl`), and the file is absent. Read-only Codex in a worktree: `printf readonly > ro.txt` failed with `operation not permitted`. Failure: a nonexistent model failed natively; the report named the kept worktree. Cancellation: the agent wrote `partial.txt`, then `StopExternalSubagent` confirmed `cancelled`; the report listed `?? sub/partial.txt` and the file was still present. Pi read-only ran with its working directory inside the worktree. The original snapshot was identical afterwards (`M sub/tracked.txt`, `?? draft.txt`, edit intact). All six worktrees remained in `git worktree list` after the server exited. |
| A07 | Open (external) | `isolation:"remote"` is rejected before any work with the external reason; no native process starts. Investigated 2026-10-21: `codex cloud` (exec/status/list/diff/apply) has no cancel command. It also runs in a Codex Cloud environment bound to a pushed GitHub branch, not this working directory; this account lists no tasks, and the CLI cannot list environments. `codex exec-server --remote` registers a WebSocket exec server that the user must provision on a remote host; none exists here, and starting one locally would only alias a local directory. Pi 0.84.4 has no remote execution. |
| A13 (isolation) | Pass | The initialization catalog, each preset's limits (`Isolation: worktree supported …; Remote: rejected …`), the tool description and the `isolation` field description all state the same support. Reports carry the worktree path, branch, base, effective writable roots, retention and the state after the last task. Isolation is fixed per ExternalSubagent: continuation reuses the same worktree. A removed worktree fails explicitly, and the task never moves to the original directory. Unknown isolation values reject. |

Note (2026-10-21, after independent acceptance of `a09587c`): the reviewer
found that a writable root spelled as a case variant of the repository
(`/…/REPO`) was treated as an unrelated root and kept. On case-insensitive
APFS, the Codex sandbox then allowed writes to the original checkout. A real
Codex run modified a tracked file (rollout exit 0, original `git status` showed
` M top.txt`). The A06 pass above did not cover that grant shape. Fix:
containment now compares directory identity (`os.SameFile` along the ancestor
chain), and an unidentifiable root rejects the call. Deterministic regression
tests cover case and Unicode NFC/NFD variants of the repository and of an
ancestor, plus the plan's effective roots; they fail on the old string
comparison. A live re-run of `make smoke-mcp-isolation-live` with an added
probe passed:
- Grant roots were `[repo, REPO]`; the effective roots were only the worktree.
- `printf escaped > …/REPO/sub/variant-escape.txt` failed with `operation not
  permitted` (rollout
  `~/.codex/sessions/2026/10/21/rollout-2026-10-21T05-31-03-01a15e71-8f99-7e70-864b-373b0bd9f8e1.jsonl`),
  and the file is absent.
- The original snapshot was unchanged, and all 7 worktrees were kept.

Evidence:
`/private/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-mcp-isolation-q97496`
(`server-debug.log`, `smoke-output.json`). The live probe covers the case
variant only; NFD is covered deterministically.

Note (2026-10-21, later the same day): the user decided on a subtraction fix
instead, and it supersedes the directory-identity mapping of `23ead9d`.
Isolation now overrides grant `writable_roots`: under `bounded-write` the
sandbox's only writable root is the worktree, and no user-written path string
reaches an isolated run. This removes the whole class of spelling variants
(case, NFC/NFD, symlink, firmlink) rather than recognising each one.

Deterministic tests use real git with a fake native runner. Grants made of a
case or NFD variant of the repository, a variant subdirectory and an unrelated
directory yield exactly `writable_roots=["<worktree>"]`, and the report states
the override. A 16-way concurrent creation test also exposed a git race:
concurrent `git worktree add` fails reading another worktree's `commondir`.
That race first appeared in this live run. Creation is now serialized
in-process; without the lock the test fails, with it the test passes.

Live re-run of `make smoke-mcp-isolation-live` passed. Evidence:
`/private/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-mcp-isolation-VR8Eqe`
(`server-debug.log`, `smoke-output.json`).
- Grant roots `[…/REPO, …/unrelated]`: the effective roots were only the
  worktree. `printf escaped > …/REPO/sub/variant-escape.txt` failed with
  `operation not permitted` (rollout
  `…/rollout-2026-10-21T05-34-05-01a15e74-55a6-71a1-af88-eb88ba917ac0.jsonl`),
  and the file is absent.
- Write inside the worktree: exit 0 (`…-01a15e74-54f8-79e2-abac-6700a4260665.jsonl`).
- Absolute write to the original: denied (`…-01a15e74-5531-7cd2-bf89-8c7da695492b.jsonl`).
- `printf broken > ../.git` against the worktree's `.git` file: denied
  (`…-01a15e74-5626-7893-986a-9938b2004758.jsonl`); the file still holds its
  `gitdir:` line.
- The original snapshot was unchanged, and all 8 agent worktrees were kept.

Enforcement under `isolation:"worktree"`:

| Aspect | Codex | Pi |
| --- | --- | --- |
| Working directory | Worktree path of the original working directory | Same |
| Writes | OS sandbox. `bounded-write`: the only writable root is the worktree (grant `writable_roots` do not apply); the worktree's `.git` stays protected. `read-only`: no writable root | No write tools (read-only only) |
| Reads | Not path-restricted: the original checkout stays readable | Not path-restricted |
| Commits | The repository's `.git` is outside the roots, so the agent cannot commit; work stays uncommitted in the worktree | Not applicable |

Deterministic coverage (`go test ./internal/subagents`, real git with a fake
native runner) covers the following:
- Root rebasing.
- Dirty-original preservation and plan cwd/roots.
- Report contents.
- Same-worktree continuation, and explicit failure when the worktree is removed.
- Retention on failure and cancellation.
- Rejection outside a git work tree, or when the working directory is untracked at HEAD. A rejected call leaves no worktree or branch behind.
- Remote and unknown values rejected without native execution.
- Pi plan cwd.

Note (2026-10-21): rows A04 and A12 of the scenario matrix below, and the
retained preset-visibility observations, predate G05. Their statement that all
isolation is rejected was true for that run. Worktree isolation is supported
since branch `feat/isolation`; remote stays rejected.

## Environment and retained observations

### Current preset DATA visibility

The config-derived catalog is shared by initialization instructions and creation
metadata/model choices. Deterministic coverage checks quoted data boundaries,
disabled/invalid/empty/no-default states and reconnect snapshot semantics.

One author-owned observation used the exact question `有哪些可用subagent选择？`,
default native tools, deferred search and `dontAsk`, with randomized isolated
presets. Claude Code 2.1.296 / `claude-fable-5-1` returned both presets with their
correct engines/models and readonly limits, distinguished native types, and
qualified static checks versus unverified remote availability. No tools were
called. Safe answer/call evidence is `observed.json` under
`/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-preset-visibility-zex6J4`.
This is one observed response, not a deterministic language-behavior guarantee.
Independent reviewer-owned Claude observations (2.1.296 / `claude-fable-5-1`)
subsequently accepted natural behavior:

- A different general question listed both randomized presets, default, readonly
  capabilities/limits and distinguished internal Agent types with zero tool calls
  (`agentx-preset-visibility-6kzu6j`).
- An exact-model/isolation question preserved both complete configured model IDs,
  explicitly rejected isolation and qualified remote availability, with zero
  tool calls (`agentx-preset-visibility-2JPu68`).
- A created/continuable-session question used ToolSearch then
  `listExternalSubagentSessions`, correctly reporting an empty connection rather
  than presenting configured presets as created instances
  (`agentx-preset-visibility-j6tjOk`).

These directories share the temporary parent path above; each contains
`observed.json`. The first two decisive observations used an isolated temporary
working directory. An earlier exact-model question from the repository directory
used Bash to inspect source (`agentx-preset-visibility-Ay2AvN`); it is excluded as
evidence that metadata alone suffices. An initial general response shortened the
Pi model ID, motivating more precise DATA labels before the accepted observations.
Reviewer semantic judgment, not a fixed keyword assertion, determined acceptance.
Independent `make check test test-mcp-race test-mcp-syntax smoke-mcp-contract
smoke-mcp-management` and `git diff --check` also passed. These samples are not a
guarantee for every future model response. The pre-rename lifecycle observations
below are not relabeled as current discovery.

| Component | Observed version / combination |
| --- | --- |
| Host | Interactive Claude Code 2.1.296, first-party provider |
| Codex | CLI 0.147.0, exact `gpt-5.6-luna`, native read-only sandbox |
| Pi | CLI 0.84.4, exact `openai-codex/gpt-5.6-luna`, readonly tool allowlist |
| MCP | stdio, legacy revision `2024-11-05`, server `agentx` |

`make smoke-mcp-live` passed with both real backends concurrently. Each first
task received a distinct public fictional fruit label. Subsequent SendMessage
recalled it without the caller repeating it:

| Backend | AgentX object | First-turn-only label returned after native resume |
| --- | --- | --- |
| Codex | `ax-bf742411ae2e3a7cca5b203b52bdd897` | `bb2f4fce-0cc4-44b4-8445-9a4331489d9f` |
| Pi | `ax-ef216c867c92b6c0418cc5428112d907` | `76e84bd6-6640-4f75-ac8a-a16a21012aea` |

Both reports were terminal `completed` with native backend usage. Codex used
explicit `low` effort. Its resume report contained `input_tokens:30463`,
`cached_input_tokens:14976`, `output_tokens:49`, `reasoning_output_tokens:11`.
Pi contained `input:2386`, `output:39`, `reasoning:12`, `totalTokens:2425`.
These retain raw native meanings, not Claude usage or normalized per-turn totals.

A separate real Codex object `ax-4d6b914f3e5f6070f8a556d2310576d1` was
cancelled. TaskStop waited for native execution to return after cancellation and
reported `cancelled`, with unknown usage. Controlled runner tests separately
cover cancellation that is ignored: a completed native result stays completed
and TaskStop returns an explicit unconfirmed-cancellation error.

Raw native command/debug evidence was retained at
`/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-mcp-live-qBrdJ6/`
(`mcp.jsonl`, `external-debug.jsonl`). Temporary raw files may expire; the
observations above and reproducible source harness are the durable record.
Debug logs contain native plans, stdin, stdout, stderr and errors.

`make smoke-mcp-channel` passed with actual interactive Claude and a delayed
deterministic Pi-shaped fixture. The fixture token was absent from the user's
task. Claude invoked Agent once, got its immediate queued response, then consumed
a separate completion channel and produced `CHANNEL_RECEIVED_8f326c` in a later
assistant message. Debug lines included:

```text
MCP server "agentx": Channel notifications registered
MCP server "agentx": notifications/claude/channel: AgentX subagent ...
```

Host transcript:
`/Users/agent/.claude/projects/-Users-agent-work-agentx/0800b2c3-e18c-4e6c-9153-b4f9492a64f7.jsonl`.
PTY/debug evidence:
`/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-channel-rlzfCJ/`.
The final harness additionally asserts the registration debug line. Channel
metadata was later renamed from `source` to `report_origin` to avoid duplicating
Claude's authoritative `<channel source="agentx">` attribute; a regression
asserts no `source` metadata key remains.

`make smoke-mcp-management` passed using the real Claude MCP configuration CLI
under isolated HOME. It preserved an unrelated server, enabled twice, disabled
and re-enabled without losing options, and rejected a conflicting user-owned
AgentX entry without mutation. Evidence:
`/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-mcp-management-aswCF4/`.
This management test did not install a live entry in normal user settings.
Native Claude startup may update its own ordinary caches.

The integrated actual-host/both-engine run established actual Claude→Codex/Pi
creation, busy SendMessage acceptance on each worker, both IDs running in the
same ListAgents observation, and both backend-only recalled labels in later
assistant output. A strict transcript parser rechecked exact tool call IDs,
names/models, result identities and actual channel attachments:

| Worker | AgentX object | Label known only to native backend until recall |
| --- | --- | --- |
| codex-worker | `ax-6e768c3ee59fd7e7b02b05c821633dfb` | `f22f62eb-5a25-4843-917e-8ce7065993b6` |
| pi-worker | `ax-9339206f5690d83c2d49028a62deaa9e` | `17662fa4-a1e6-434a-b5a0-562c371ad9b9` |

Neither first-turn label appeared in the parent task; each was read by its native
backend from a separate public test-data file. The parsed parent assistant made
no Read/Bash call. Exactly one Agent and one busy SendMessage per named worker
were observed. Both completed-recall channel attachments and final assistant
labels were verified. Evidence directory:
`/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-channel-1FfUv1/`;
transcript:
`/Users/agent/.claude/projects/-Users-agent-work-agentx/724aadfa-c0d2-4c5b-9fb0-204fb2dc80cc.jsonl`.
`make verify-mcp-host-evidence` passed with these paths in `MCP_EVIDENCE_DIR` and
`MCP_TRANSCRIPT`. The original combined run's extra cancellation target finished
naturally before TaskStop, so that particular cancellation assertion failed and
is not counted as a successful cancellation. The maintained host-live harness
now concentrates on host/both-engine/busy-continuation delivery.

The extended `make smoke-mcp-live` subsequently passed real concurrent
cancellation independently: ListAgents showed Pi survivor
`ax-106e8abb6796eb8260b597a53a00bad2` and Codex target
`ax-85a279c8e29cbd2b6fc6fffbbf42907b` both `running`; TaskStop reported only
the target `cancelled`; Pi later completed with `SURVIVOR_COMPLETED`.
Evidence:
`/var/folders/34/yf5zv4k133l7nrksmz2flmw00000gn/T/agentx-mcp-live-tc7FXh/`.
This run also reverified real memory continuation on both engines. Cancellation
and host notification claims are supported by their respective actual runs,
not inferred from an all-mock suite or from the failed natural-finish attempt.

## Scenario matrix

“Deterministic” means controlled Go runner tests. “Live” means installed native
CLI/host behavior. Injection does not establish a real provider outage; policy
checks are not a sandbox penetration test.

| Scenario | Evidence / observation |
| --- | --- |
| A01 First use | Live isolated management installs persistent MCP entry. Actual Claude discovers and invokes both configured real engine choices without registration. |
| A02 Single call | Actual Claude creates each real backend object in one Agent call. Separate live foreground calls return terminal native reports, identities and hints. |
| A03 Defaults | Historical (pre-002): configured-default execution. Since 002 there is no default preset; an omitted `model` is rejected (deterministic server test). |
| A04 Constraints | Unknown/disabled choices and unsupported role/effort/isolation reject; capabilities/limits visible in generated tool description. |
| A05 Fore/background | Live both engines false waits; real Claude true returns queued then consumes channel. Deterministic omitted background returns queued immediately. |
| A06 Memory | Live distinct first-turn labels recalled via both native sessions, no caller history replay. |
| A07 Busy message | Actual Claude's exact SendMessage results report each matching real worker running and message accepted/queued; both later recall their own context through channel results. |
| A08 Parallel/addressing | Live different engines parallel with distinct contexts; deterministic same-combination fresh sessions, name reassignment and old-ID access. |
| A09 Cancellation | Real ListAgents shows Pi survivor and Codex target running together; cancelling Codex terminates it while Pi later completes. Deterministic per-queued-task notifications and unknown/already-ended cases supplement this. |
| A10 Disable/re-enable | Live isolated management preserves the presets collection (`make smoke-mcp-management`); deterministic new-work rejection and completion of accepted queue while disabled. |
| A11 Configuration changes | Each new task reads current config. Original option pinned at acceptance/dequeue; changed or removed identity rejects continuation. |
| A12 Parameters | Deterministic required/optional/unknown/null/wrong-type/case/empty checks. Deprecated mode/team_name cannot alter readonly native plan. Live Codex low effort; Pi effort and all isolation reject. |
| A13 Failures | Injected unavailable-model/execution failure is a tool error. Malformed/incomplete output, missing Pi session, changed native ID fail without retry. Ignored cancellation is explicitly unconfirmed, not cancelled. |
| A14 Usage/source | Live raw usage or unknown; labelled untrusted external reports; native plans call selected Codex/Pi, no Claude relay. |
| A15 Documentation | Management, fields, native constraints, host gates, connection scope and test commands documented. Only listed models have actual native inference evidence. |

## Requirement mapping

| Requirements | Implementation / checks |
| --- | --- |
| U01–U05 | Persistent strict user configuration; management and exact-entry MCP registration; connection IDs/clientInfo/initialized status identify current host separately from channel gate. Live management/discovery. |
| U06 | Explicit default or model-required error; default/no-default regression. |
| U07–U08 | Gate read on every new Agent/SendMessage; accepted queue may finish; TaskStop works disabled. |
| U09 | Original option/model/native session pinned; configuration/session mismatch rejected. |
| U10 | Distinct queued/running/completed/failed/cancelled states; failed/cancelled foreground tool errors. |
| C01–C04 | Four MCP tools; familiar exact Agent fields; model contains configured combination; visible strengths and limits; no registration or extra agent field. |
| C05–C08 | General-purpose role independent of model; no forced orchestration; strict inputs; unsupported values reject; deprecated fields never grant permission. |
| L01–L04 | Fresh objects, immediate backgrounds, terminal foregrounds, exact native resume; live memory and host channel consumption. |
| L05–L07 | FIFO messages and per-task terminal notifications; native completed-object resume; names/IDs and name reassignment. |
| L08–L10 | Targeted process-group cancellation waits; unconfirmed cancellation honest; independent objects; lost session never silently recreated or replayed. |
| R01–R03 | Readable reports, provenance and interaction hints; injected/native failures distinct from successful reports. |
| R04–R05 | Native external usage or unknown; CLI delegation with no Claude relay. |
| R06–R08 | Native readonly policy/allowlist; rejected unsupported settings; no permission inheritance claim/fallback; acceptance/running/terminal states distinct. |

## Explicit limits

- Background delivery is verified for interactive first-party Claude Code
  2.1.296 with allowed channel policy. Headless print did not register this
  handler; use foreground there. No organization/feature policy is bypassed.
- MCP has no channel presentation acknowledgement. Status identifies live
  initialized connections, but cannot certify their host UI gate. Actual
  completion evidence independently establishes that path.
- General-purpose, local native execution under each grant, worktree isolation
  and configured Codex effort are supported. Pi explicit effort, remote
  isolation (004 A07, no verifiable remote environment with cancellation), other
  MCP engine adapters and cross-connection recovery are unsupported and
  rejected/reported.
- Pi's allowlist is not an OS sandbox. Pi `bounded-write` is rejected; a real
  Pi write boundary (004 G02 for Pi) remains open. Approve and deny approval
  scenarios (004 A03) remain pending: only `approval:"never"` exists, and host
  approvals are not inherited. Native context does not inherit Claude
  permissions, and authentication comes only from each grant's source.
  Deprecated mode never grants extra access.

Reproduce deterministic/management checks with
`make verify test-mcp-race smoke-mcp-management`. Paid native/host checks use
`make smoke-mcp-live smoke-mcp-channel smoke-mcp-host-live smoke-mcp-policy-live smoke-mcp-isolation-live`
and the logins selected by each grant's auth source.
