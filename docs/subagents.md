# AgentX MCP ExternalSubagents

AgentX exposes `ExternalSubagent`, `SendExternalSubagentMessage`, `StopExternalSubagent`, and `listExternalSubagentSessions` over stdio MCP.
These are explicitly AgentX tools, not official Claude subagents. The selected
native agent executes directly, without a Claude relay inference call. Claude chooses
the combination and any orchestration; AgentX imposes no review/voting workflow.

## Presets: one collection for `ax` and MCP

A preset is a named, user-confirmed choice of agent, model and effort plus a
description of when to use it. The same collection serves the bare `ax` home
screen and the `ExternalSubagent` choices; there is no separate MCP list.

```text
~/.agentx/presets/
  deep-review/
    PRESET.md
  daily/
    PRESET.md
```

The directory name is the preset name (an ASCII letter or digit, then letters,
digits, `_` or `-`, at most 64 characters). `PRESET.md` holds a front matter
block and the description:

```markdown
---
agent: codex
model: gpt-5.6-luna
effort: low
---

Read-only research and code review. Not for large refactors.
```

- `agent`: `claude`, `codex`, `gemini`, `opencode` or `pi`. Only `codex` and
  `pi` can run as MCP `ExternalSubagent`. Other presets appear in `ax` only;
  the MCP catalog, tool index and `status` leave them out, and a call naming
  one is rejected as `interactive launch only`.
- `model`: the exact native model ID, passed unchanged to `--model`. Pi needs
  `provider/model`. No fallback model is ever used.
- `effort` (optional): omitted means the agent's native default and is shown as
  "native default". Explicit levels: Codex `none`, `minimal`, `low`, `medium`,
  `high`, `xhigh`, `max`, `ultra` (the creation flow offers only the levels Codex
  lists for the chosen model; a manually entered model shows them as
  unverified); Pi `low`, `medium`, `high`, `xhigh`, `max` (`--thinking`);
  Claude `low`, `medium`, `high`, `xhigh`, `max` (`--effort`; per-model support
  unverified). Gemini and OpenCode accept no explicit effort. Pi `off` and
  `minimal` are rejected: for Codex-backed models Pi sends no effort for `off`
  (the server default, medium for gpt-5.6-luna, applies) and sends `low` for
  `minimal`. As an ExternalSubagent, every turn with an explicit effort is
  checked against native evidence (see
  [Effort verification](#effort-verification)); interactive launch passes the
  level but cannot check it.
- The description is the Markdown body. `name` and `description` keys in the
  front matter and unknown keys are rejected, so there is one authority.

`AX_SUBAGENT_HOME` replaces `~` for both the collection and the AgentX MCP
configuration/state in isolated environments; it does not change native
credentials.

### Create and manage from `ax`

Running `ax` with no arguments shows **New session · Presets** first, with
`+ Create preset…` as its last entry, then recent sessions for the current
workspace and globally. Enter on a preset starts a new native session in the
current directory with exactly that agent, model and effort; Enter on a recent
session continues it as before. `ax <agent>` direct launch is unchanged.

`+ Create preset…` asks for the agent, the model (listed when the agent exposes
a reliable list; otherwise the limitation is shown and the exact ID is typed),
the effort, a name and a description, then shows a summary with
`Save and start`, `Save`, `Back` (change any entry) and `Cancel`. Saving writes
the directory atomically and runs no inference; listing models only reads model
metadata. Duplicate or invalid names are rejected with the other entries kept.
Cancelling leaves nothing behind.

Press `m` on the home screen to manage the highlighted preset (or choose one):
View (configuration and availability status separately), Edit, Rename,
Duplicate (an independent copy, no inheritance), Delete (asks for the named
preset; never touches sessions, native agents, credentials or running work) and
Start new session. Invalid entries are listed with their error and file
location, can be repaired or deleted, and never hide other presets or recent
sessions. Hand edits to `PRESET.md` appear on the next screen or MCP request.

If a native agent is missing at start, AgentX shows the reason and offers to edit
the preset or go back; it never substitutes another agent, model or effort.
Native rejection of a model or effort is the native agent's error.

### Presets and the AgentX server

The presets collection (`~/.agentx/presets/`) is the whole configuration. There is no AgentX switch or
permission file. Agents launched through `ax` (`ax claude`, `ax codex`) get the
AgentX MCP server and the ExternalSubagent tools automatically. A plain `claude` started without `ax` gets them only if you register the server yourself: `claude mcp add --scope user agentx -- "$(command -v ax)" mcp serve`.

```sh
ax preset list
ax preset show codex-review
ax claude
```

- `ax preset list` shows the collection with static readiness, limits and
  invalid entries; `ax preset show <name>` shows a single preset's detail.
  Static checks do not verify login or model access; those are established per
  execution. No native-default model or automatic fallback is used.

New work reads the current collection. Existing ExternalSubagents retain their
original combination: an edited, renamed or deleted preset rejects
continuation and queued work instead of rebinding it, and running work is never
stopped or switched by a change.

### Migrating from `subagents.json`

`~/.config/agentx/subagents.json` (its `enabled` switch, the earlier
`default`/`options` keys and the later `grants`) is no longer read. AgentX
ignores the file if it exists and never rewrites it; you can delete it. To
migrate:

- Move each former option into its own preset once (a former option with
  several `efforts` becomes one preset per effort you use; a former
  `enabled: false` option is simply not moved):

  ```sh
  mkdir -p ~/.agentx/presets/codex-review ~/.agentx/presets/pi-review
  printf -- '---\nagent: codex\nmodel: gpt-5.6-luna\neffort: low\n---\n\nResearch and code review using native Codex.\n' > ~/.agentx/presets/codex-review/PRESET.md
  printf -- '---\nagent: pi\nmodel: openai-codex/gpt-5.6-luna\n---\n\nIndependent research and code review using Pi.\n' > ~/.agentx/presets/pi-review/PRESET.md
  rm ~/.config/agentx/subagents.json
  ```

- A former `enabled: true` needs no replacement: `ax claude` and `ax codex`
  attach AgentX; the earlier `enable`/`disable` subcommands no longer exist.
- A former grant's `bounded-write` with `writable_roots` has no replacement:
  what an agent may write is decided by its own native configuration, or
  bypassed per call with `mode: "yolo"`; see
  [Permission mode and isolation](#permission-mode-and-isolation).
  Its `approval` has no replacement because `never` was the only value.
- A former grant's `auth.path` becomes the native agent's own variable
  (`CODEX_HOME`, `PI_CODING_AGENT_DIR`) in the environment AgentX runs in, or
  nothing when it was the default directory.

## Permission mode and isolation

Permissions belong to the native agent, not to AgentX. Each `ExternalSubagent`
call picks one `mode`, fixed for that ExternalSubagent:
`SendExternalSubagentMessage` and queued work reuse it and cannot change it.

| `mode` | Claude | Codex | Pi |
| --- | --- | --- | --- |
| `normal` (default, also when omitted) | No permission flags: Claude's own settings decide | Only `approval_policy="never"`: the sandbox is whatever the user's Codex configuration sets | Read-only tool allowlist |
| `yolo` | `--dangerously-skip-permissions` | `sandbox_mode="danger-full-access"` plus `approval_policy="never"` | Rejected before any work starts |

- `normal` is a passthrough. AgentX injects no sandbox, approval or permission
  flag, except `approval_policy="never"` for Codex: a headless run cannot answer
  an approval prompt. Claude in print mode refuses tools its settings do not
  allow; that refusal is Claude's own behavior.
- `yolo` bypasses the agent's permission checks and sandbox entirely. Use it
  only where the agent may do anything the user could.
- Pi has no runtime permission boundary to bypass, so `yolo` is rejected for
  Pi presets. Pi always runs with its read-only tool allowlist.
- `isolation` is independent of `mode`: it only chooses where the agent works.
  See [Isolation](#isolation).

The preset's `user_config` and `flags` are passed through unchanged. With
`user_config: false` Codex runs with `--ignore-user-config --ignore-rules
--disable hooks`, so MCP servers, hooks and `notify` programs from the user's
`config.toml` are not loaded; user-config defaults such as model effort,
personality and service tier then also do not apply.

### Authentication

Authentication is discovered, not configured. Before every turn AgentX selects
the native agent's own credential directory, which is `CODEX_HOME` or
`PI_CODING_AGENT_DIR` from its environment, otherwise `~/.codex` or
`~/.pi/agent` (`AX_SUBAGENT_HOME` never changes it), and pins the native
process to it through that variable. Each report carries an
`Auth source for this turn:` line with the variable and directory. AgentX runs
no login or readiness check of its own: whether the directory can serve the
preset's model is decided by the runtime, which resolves models and credentials
with its own rules and reports a failure in its native stream. A relative
`CODEX_HOME` or `PI_CODING_AGENT_DIR` fails before any work starts.

Inherited environment variables whose names look like credentials (`*KEY*`,
`*TOKEN*`, `*SECRET*`, `*PASSWORD*`, `*CREDENTIAL*`, `*AUTH*`, `*COOKIE*`,
`AWS_PROFILE`) are removed from the native process. Without this, Pi would
silently use an inherited `OPENAI_API_KEY` when the directory has no login.
AgentX never copies credentials between runtimes. Identity means the directory;
the login method, provider and account inside it are not checked.

## Claude discovery and background delivery

MCP initialization includes a preset DATA catalog read from the collection:
names, execution agents, exact models, effort, the user's descriptions, whether
each preset is callable as ExternalSubagent, its mode and auth behavior,
static prerequisites, and invalid entries with their
location. The same collection supplies the selectable `model` values (callable
presets only). `ExternalSubagent` tool metadata
carries only a compact index (one line per preset: agent, model,
effort, callability, description) because hosts truncate tool descriptions
(Claude Code keeps 4096 characters); limits are only in
the initialization data. With no presets the catalog says so and that the user
creates them in `ax`; none are generated. Static checks do not verify login or
model access. Session listing reports created objects, not presets.
A call result notes when presets changed since initialization.
Initialization instructions are a connection snapshot. After saving, renaming or
deleting a preset, `ax` says whether running AgentX MCP connections still show
the old list and to reconnect (`/mcp`) or restart Claude. New calls always use the saved preset; a call
result notes when the collection changed since the connection initialized. A
fresh `tools/list` reads the current collection.

Restart or reconnect the host for tool discovery. `ax claude`
adds the MCP connection and allows exactly the four AgentX tools for that session
with `--allowedTools`; Claude's persistent `settings.json` is not modified.
Background delivery is file based and host agnostic. When a task ends
(completed, failed or cancelled, including queued work cancelled before it
started), AgentX atomically writes one JSON document to:

```text
~/.local/state/agentx/connections/<connection-id>/jobs/<task-id>.json
```

The document is `{"content": <rendered report>, "meta": {"report_origin":
"agentx-subagent", "agent_id", "task_id", "status"}}`. The immediate response of
`ExternalSubagent` (background) and `SendExternalSubagentMessage` includes the
absolute path and a wait command:

```sh
until [ -f "<abs-path>" ]; do sleep 1; done; cat "<abs-path>"
```

Run it in the background (Claude Code: `Bash` with `run_in_background: true`) so
the host is woken once with the report. Every follow-up message has its own task
ID and result file. Each tool response and report includes a connection ID. Match
it to `status.connections` to identify the current host, its client name/version
and whether it sent `notifications/initialized`. Foreground calls
(`run_in_background:false`) wait and return the terminal result directly.
Set `AX_LOG=debug` to log each result file path and write error to stderr.

## Tool contract

```text
ExternalSubagent({"description":"Review authentication", "prompt":"Review token refresh logic",
       "model":"codex-review", "name":"reviewer", "run_in_background":true})
ExternalSubagent({"description":"Fix the flaky test", "prompt":"Fix and run the test",
       "model":"codex-review", "isolation":"worktree", "mode":"yolo"})
SendExternalSubagentMessage({"to":"reviewer", "message":"Now inspect logout handling", "summary":"Check logout"})
StopExternalSubagent({"task_id":"reviewer"})
listExternalSubagentSessions({})
```

`description` and `prompt` are required strings. `model`
is required and names one callable preset; there is no default preset. The preset
fixes agent, model and effort; there is no `effort` parameter and no per-call
override. Reports name the preset, agent, actual model and effort, and a
`Native evidence:` line states the executed effort that the
native records show for that turn. `isolation` accepts only `worktree` (see
below); any other value is rejected before any work starts. `mode` is
`normal` (default) or `yolo`; see
[Permission mode and isolation](#permission-mode-and-isolation). Reports carry a
`Mode:` line and the discovered `Auth` line. Names start with an ASCII
letter or digit and contain 1–64 ASCII letters, digits, `_`, or `-`.
Deprecated `team_name` is accepted and ignored. Unknown properties (including
the removed `write` and `writable_roots`), nulls, wrong types, and unsupported
values are rejected.

Omitted/true `run_in_background` returns a task identity immediately and pushes
its terminal result. False waits for a terminal result; failure/cancellation is
a tool error, not successful submission. `SendExternalSubagentMessage` accepts text and returns
queued acknowledgement, never a claim of completion. Busy ExternalSubagents process
messages in order using the same native context. Every accepted message has its
own task ID and terminal notification, including cancellation or prior-turn
failure. Completed ExternalSubagents resume via their exact native session.

Every ExternalSubagent call creates an independent object. Reusing a name makes that name
refer to the newest object; previous objects remain addressable by ID. Cancel
targets only that object's process group and queued work, waits for termination,
and distinguishes already-finished and unknown targets. Failed/cancelled agents
cannot resume an uncertain context; the error is explicit and no replacement
session or repeat task is created.

ExternalSubagents are session-scoped, like Claude Code's native subagents: they
belong to the MCP connection that created them, and listExternalSubagentSessions
shows only those. A reconnect, or a resumed parent Claude session, starts a new
connection with no ExternalSubagents; an old name or ID then fails explicitly
as unknown. Native transcript files may remain on disk, but AgentX does not
restore objects across connections, by design. Session loss or identity
mismatch is failure, not retry.

## Isolation

`isolation:"worktree"` runs the ExternalSubagent in a new git worktree. It
only chooses the working directory; it grants and restricts nothing, and works
the same in either `mode`:

- **What is created.** The repository is the one containing the MCP server's
  working directory, and the worktree starts at its `HEAD` commit. It lives at
  `~/.local/state/agentx/worktrees/<id>` on branch `agentx/<id>`, where `<id>`
  is the ExternalSubagent ID. The native agent's working directory is the
  matching path inside the worktree.
- **The original checkout.** AgentX never modifies its files. Its uncommitted
  and untracked changes are not copied into the worktree. Whether the agent
  itself can reach the original by absolute path is decided by the agent's
  native configuration (`normal`) or not restricted at all (`yolo`). The original repository
  does gain branch `agentx/<id>` and git's worktree metadata under
  `.git/worktrees/`.
- **Rejections.** The call is rejected before any work starts in three cases:
  the working directory is not inside a git work tree, there is no commit at
  `HEAD`, or the working directory is untracked at `HEAD` and therefore
  missing from the worktree. A rejected call leaves no
  worktree or branch behind.
- **Repository hooks.** `git worktree add` runs the repository's own
  `post-checkout` hook, as git always does. That hook is the repository's code,
  not the agent's. It runs outside any agent sandbox, with AgentX's full
  inherited environment, credential-like variables included. Choose worktree
  isolation only for repositories whose hooks you trust.
- **Why the worktree is outside the repository.** A nested worktree would show
  up as an untracked entry in the original checkout, and `git add -A` there
  would record it as an embedded repository.
- **Concurrent creation.** Worktree creation is serialized within one AgentX
  process, because concurrent `git worktree add` in one repository fails
  (git 2.39 reads another worktree's half-written metadata). Separate AgentX
  processes on the same repository can still hit this; the call then fails
  explicitly with no task started.
- **Pi.** For Pi a worktree changes only the working directory.

The worktree is fixed for the ExternalSubagent's lifetime. Continuation reuses
it and cannot rebind it. If the worktree has been removed, continuation fails
and never falls back to the original directory.

Retention: AgentX never removes a worktree, whether the task completed, failed
or was cancelled, and including after the MCP connection ends. This includes a
task that failed before native execution started: the worktree was already
created, and the failure report names the worktree path. Continuation
needs the worktree, and any uncommitted work in it belongs to the user. Every
report names the path, branch, base commit and mode. After
each task, the report also records the state: whether `HEAD` moved, plus
`git status --porcelain`. It ends with the cleanup command:
`git -C <repo> worktree remove <path>` (which refuses while changes remain; add
`--force` to discard them), then `git -C <repo> branch -D agentx/<id>`. To take
the agent's changes, inspect or copy them from the worktree, or commit them
there yourself.

Bulk cleanup. Retained worktrees accumulate until you remove them. To list
them:

```sh
git -C <repo> worktree list                   # paths and branches, incl. AgentX ones
git -C <repo> branch --list 'agentx/*'        # AgentX branches
ls ~/.local/state/agentx/worktrees            # worktrees from every repository
```

Inspect each one before removing it. Then run `git -C <repo> worktree remove
<path>` and `git -C <repo> branch -D agentx/<id>` for each. If a worktree
directory was deleted by hand, run `git -C <repo> worktree prune` to drop its
stale metadata. The branch stays until you delete it.

## Nested delegation

A dispatched Codex ExternalSubagent automatically has AgentX available as an
MCP server and can call `ExternalSubagent` itself. This gives dispatched agents
the ability to delegate subtasks to other agents without user configuration.

- **Codex only.** AgentX injects its own MCP server configuration into the
  Codex `-c` flags alongside `--ignore-user-config`, so the only MCP server
  available to the dispatched Codex is AgentX. Pi has no MCP server support.
- **Depth limit.** Delegation depth is tracked via `AX_DELEGATION_DEPTH` and
  capped at 2. A depth-0 server (the top-level connection from Claude Code)
  injects depth 1; a depth-1 server injects depth 2; a depth-2 server does
  not inject further. The initialization instructions report the current depth
  and whether further delegation is available.
- **Foreground only.** Nested hosts (Codex exec) do not wait on result files, so
  `run_in_background` is forced to false regardless of the caller's request.
  The dispatched agent waits for each sub-agent to complete before continuing.
- **Independent mode.** Each nested ExternalSubagent call picks its own
  `mode`, the same as a top-level call. A `normal` parent does not prevent a
  `yolo` nested call (same as how Claude Code orchestrates top-level calls).
- **Independent auth.** The nested AgentX discovers credentials per execution
  through the same `CODEX_HOME` / `PI_CODING_AGENT_DIR` mechanism. Credential
  environment variables are stripped as usual.
- **`AX_SUBAGENT_HOME`.** If set in the parent process, it is propagated to
  the nested MCP server so the same preset collection is used.

## Native execution and permission limits

| Adapter | Boundary | Native continuation | Effort |
| --- | --- | --- | --- |
| Claude | `normal`: no permission flags (Claude's own settings); `yolo`: `--dangerously-skip-permissions`; native auth | Exact session UUID (`--resume`) | Preset effort via `--effort`, or native default |
| Codex | `normal`: only `approval_policy="never"` (sandbox from the user's Codex config); `yolo`: `sandbox_mode="danger-full-access"` plus `approval_policy="never"`; `user_config: false` skips user config/rules/hooks; discovered `CODEX_HOME` | Exact thread UUID | Preset effort via `model_reasoning_effort`, or native default; each turn checked against `response.completed` telemetry; native errors remain errors |
| Pi | Read-only only, `yolo` rejected: `--tools read,grep,find,ls --no-extensions --no-skills --no-prompt-templates --no-approve`, discovered `PI_CODING_AGENT_DIR`; with an explicit effort, only AgentX's observer extension is added (`--extension`) | Dedicated exact session path and pinned native ID | Preset effort via `--thinking` (`low`..`max`), or native default; each turn checked against the session thinking level and every provider request |

### Effort verification

Native CLIs can run a different effort from the one requested without failing,
and their own records may still show the requested one. AgentX therefore
checks what was actually sent or run, per turn, and fails the turn on any
difference. The failed report still carries the agent's text, labelled
untrusted, and the ExternalSubagent cannot be continued; nothing is retried.

- Codex: the configured level is echoed in the rollout `turn_context` even
  when Codex sends another one (`gpt-5.6-luna` with `ultra` sends `max`). AgentX
  starts Codex with `RUST_LOG=codex_otel.trace_safe=info` and reads every
  `codex.sse_event` `response.completed` event, whose `model_reasoning_effort`
  is the level the model response ran with. These trace-safe events carry no
  account or e-mail; they are removed from failure text returned to Claude and
  appear only in the debug log. A level the server does not support fails
  natively (`minimal` on gpt-5.6-luna: HTTP 400, returned as the turn's reason).
  With no explicit effort the executed level is only reported.
- Pi: Pi clamps a level the model cannot use (recorded in the session as
  `thinking_level_change`) and its providers may then remap or drop it
  without any session trace (Codex-backed models: `minimal` is sent as `low`,
  `off` sends no effort). AgentX loads its own observer extension, which only
  appends an `agentx-provider-request` entry with the request's reasoning
  effort to the Pi session before each provider request. A turn passes only
  if the session's thinking level equals the preset effort and every provider
  request of that turn carried exactly that effort. Models whose requests carry
  no reasoning effort field therefore fail explicit effort.

Verified per level on 2026-10-21 (Codex 0.163.0, Pi 0.84.4, see
[acceptance evidence](subagents-acceptance.md)):

| Combination | Runs as | Rejected |
| --- | --- | --- |
| Codex `gpt-5.6-luna` | `none`, `low`, `medium`, `high`, `xhigh`, `max` | `minimal` (native 400), `ultra` (silently run as `max`; detected) |
| Pi `openai-codex/gpt-5.6-luna` | `low`, `medium`, `high`, `xhigh`, `max` | `off`, `minimal` at preset validation (not sent as requested) |
| Pi `deepseek/deepseek-flash` | not claimed | `max` clamped by Pi to `high`; detected |

Other models are checked the same way at run time; no level is claimed for
them in advance.

Pi's allowlist is an application-level tool filter, not an OS sandbox.
Native project context (for example AGENTS.md) still applies. Credentials
and policies are not inherited from Claude. Other
agents (including Gemini) remain available for ordinary AgentX launch/exec but
are not delivered MCP combinations in this integration.

Reports identify preset, execution agent, actual selected model, effort, mode, discovered auth, ExternalSubagent/task
IDs, status, failure reason, and continuation/cancellation instructions. Reports
are labelled untrusted ExternalSubagent content and never user
authorization. Available native usage is shown as backend-reported external
usage with native field names; unknown usage stays unknown. Codex resume usage
may include context/cumulative fields; AgentX does not reinterpret it as Claude
usage or fabricate tool counts. `AX_LOG=debug` logs raw command plans, stdin,
stdout, stderr and errors to stderr; it may contain task contents. Values of
credential-like plan environment variables are logged as `<redacted>`, and
`sk-…` keys, JWTs, bearer tokens and JSON token/key fields in logged I/O are
masked. Removed variables are logged by name only (`unset_env`).

## Verification

```sh
make verify
make test-mcp-race
make smoke-mcp-management
make smoke-mcp-contract
make smoke-presets
make smoke-mcp-live
make smoke-mcp-jobfile
make smoke-mcp-effort-live
make smoke-mcp-isolation-live
```

The live backend suite uses existing native logins, isolated AgentX configuration,
and Codex `gpt-5.6-luna` / Pi `openai-codex/gpt-5.6-luna`. Override
`AX_MCP_CODEX_MODEL` / `AX_MCP_PI_MODEL` with exact accessible models. It verifies
native inference, independent memory continuation and actual cancellation. The
job-file smoke starts a background task against a deterministic fixture over MCP
stdio and asserts the result file appears with the expected fields.

Live smokes that read Codex rollouts (`$CODEX_HOME/sessions/**/rollout-*-<thread>.jsonl`)
do so because Codex code mode keeps nested shell and patch calls out of
`exec --json`. Override `AX_MCP_CODEX_HOME` and `AX_MCP_PI_AGENT_DIR` to
select the credential directories the live smokes pass to the server as
`CODEX_HOME`/`PI_CODING_AGENT_DIR` (defaults: `~/.codex`, `~/.pi/agent`).

`smoke-mcp-effort-live` runs every Codex level and Pi
`low`..`max` on gpt-5.6-luna, a Pi preset that Pi clamps
(`AX_MCP_PI_CLAMP_MODEL`, default `deepseek/deepseek-flash`, whose credential
must be in the same Pi agent directory), and direct Pi `off`/`minimal` runs with the observer to show why those
stay rejected. Evidence is written to `evidence.json` in the printed directory.

`smoke-mcp-isolation-live` starts from a dirty original checkout and runs real
Codex and Pi with `isolation:"worktree"`. It checks a `yolo` write inside the
worktree, a `normal` run in a worktree that carries no sandbox flag, native
failure, and cancellation after partial output. It also checks Pi's working
directory and that an isolation value other than `worktree` is rejected.
Finally it asserts that the original checkout's status and file contents are
byte-identical and that every worktree is kept.

See [acceptance evidence](subagents-acceptance.md) for scenarios and limitations.

The four names above replace the earlier MCP `Agent`, `SendMessage`, `TaskStop`
and `ListAgents` names. Old names are rejected; there are no compatibility aliases.
The lifecycle is unchanged; `model` now names a preset
from the shared collection and the per-call `effort` key was removed (see
Presets above). Reconnect Claude MCP after upgrading
to refresh tool metadata; update any explicit MCP tool permission rules to the
new names. Native Claude tools retain their own names and behavior.

Saved host transcripts from before this rename remain historical evidence.
To inspect one, pass `MCP_EVIDENCE_FORMAT=historical` to
`make verify-mcp-host-evidence`; the default parser and current smoke harnesses
use the new names. No new live model run is implied by renaming a harness.
`smoke-mcp-contract` checks the built binary over stdio without creating work;
set `AX_MCP_BINARY` to verify an installed binary through the same Make target.

`make smoke-mcp-presets` records an isolated real Claude natural-language
observation with randomized preset names. Override `MCP_PRESET_PROMPT` for
reviewer-owned questions; default judgment records content without a prescribed
answer. The host starts in the temporary directory, not the source repository
(`MCP_PRESET_CWD` can override it). Only ToolSearch and readonly external-session
listing calls are accepted.
`MCP_PRESET_JUDGMENT=presets` enables an optional fixed preset-answer diagnostic,
not a substitute for semantic review. Evidence records host version/model,
answer and tool calls; raw private logs stay in the printed temporary directory.
