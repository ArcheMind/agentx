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
  `pi` can run as MCP `ExternalSubagent`; other presets are listed as
  interactive launch only and are never offered as callable.
- `model`: the exact native model ID, passed unchanged to `--model`. Pi needs
  `provider/model`. No fallback model is ever used.
- `effort` (optional): omitted means the agent's native default and is shown as
  "native default". Explicit levels: Codex `none`, `minimal`, `low`, `medium`,
  `high`, `xhigh`, `max`, `ultra` (the creation flow offers only the levels Codex
  lists for the chosen model; a manually entered model shows them as
  unverified); Claude `low`, `medium`, `high`, `xhigh`, `max` (`--effort`;
  per-model support unverified). Gemini, OpenCode and Pi accept no explicit
  effort (Pi may silently clamp thinking levels).
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

### Enable MCP

```sh
ax sub-agent-management enable
ax sub-agent-management status
ax claude
ax sub-agent-management disable
```

`~/.config/agentx/subagents.json` holds only `{"enabled": true|false}`.
`status` lists the collection with static readiness, invalid entries and live
MCP connections. Static checks do not verify remote login or model access;
those are established only by execution.

### Migrating from the earlier `options` configuration

The earlier `default`/`options` keys in `subagents.json` and the `configure`
command no longer exist; a file containing them is reported as invalid. Move each
former option once into the collection and reduce the file to the switch:

```sh
mkdir -p ~/.agentx/presets/codex-review ~/.agentx/presets/pi-review
printf -- '---\nagent: codex\nmodel: gpt-5.6-luna\neffort: low\n---\n\nRead-only research and code review using native Codex.\n' > ~/.agentx/presets/codex-review/PRESET.md
printf -- '---\nagent: pi\nmodel: openai-codex/gpt-5.6-luna\n---\n\nRead-only independent research and code review using Pi.\n' > ~/.agentx/presets/pi-review/PRESET.md
printf '{"enabled": true}\n' > ~/.config/agentx/subagents.json
```

A former option with several `efforts` becomes one preset per effort you use.

Enable uses Claude's user-scoped MCP configuration command and preserves other
entries. An identical AgentX entry is reused; a conflicting entry is rejected.
Enable/disable state persists across tasks and sessions. New work reads the
current collection. Existing ExternalSubagents retain their original
combination: an edited, renamed or deleted preset rejects continuation and
queued work instead of rebinding it, and running work is never stopped or
switched by a preset change.

Disabling takes effect for new `ExternalSubagent` and `SendExternalSubagentMessage` submissions immediately.
Previously accepted work, including queued messages, can finish and deliver
results. `StopExternalSubagent` remains available. Disabling does not affect Claude native
subagents or cancel work. Re-enabling keeps the presets.

## Claude discovery and background delivery

MCP initialization includes a preset DATA catalog read from the collection:
names, execution agents, exact models, effort, the user's descriptions, whether
each preset is callable as ExternalSubagent, readonly limits, feature enablement,
static prerequisites, and invalid entries with their location. The same catalog
supplies `ExternalSubagent` metadata and its selectable `model` values (callable
presets only). With no presets the catalog says so and that the user creates them
in `ax`; none are generated. Static checks do not verify remote availability.
`model` selects a preset independently of the `subagent_type` role; session
listing reports created objects, not presets.
Initialization instructions are a connection snapshot. After saving, renaming or
deleting a preset, `ax` says whether running AgentX MCP connections still show
the old list and to reconnect (`/mcp`) or restart Claude. New calls always use
the saved preset; a call result notes when the collection changed since the
connection initialized. A fresh `tools/list` reads the current collection.

`enable` installs an `agentx` stdio MCP entry pointing to this `ax` executable.
Restart or reconnect the host for tool discovery. `ax claude` automatically adds
the MCP connection and opts in to **only** `server:agentx` using Claude's
`--dangerously-load-development-channels` flag. Claude asks for acknowledgement
of its local-development channel warning. Direct Claude invocation needs:

```sh
claude --dangerously-load-development-channels server:agentx
```

The channel path requires interactive Claude Code, first-party provider access,
host feature approval, and organization approval where applicable. AgentX
negotiates MCP `2024-11-05` (legacy); modern protocol revisions do not support
this unsolicited path in Claude Code 2.1.296. Tool discovery alone does not prove
channel delivery. `status` distinguishes persistent enablement, live MCP server
connections, static native prerequisites, and unverified host channel delivery.
Each tool response and background report includes a connection ID. Match it to
`status.connections` to identify the current host, its client name/version and
whether it sent `notifications/initialized`. A running server without that
handshake is not reported as an initialized host.
Claude's debug log should show `Channel notifications registered`; an actual
completion report confirms delivery. Headless/print Claude does not register
this interactive channel handler: use `run_in_background:false` there. Other MCP
clients may consume the named notification themselves; background push in other
hosts is not claimed.

## Tool contract

```text
ExternalSubagent({"description":"Review authentication", "prompt":"Review token refresh logic",
       "model":"codex-review", "name":"reviewer", "run_in_background":true})
SendExternalSubagentMessage({"to":"reviewer", "message":"Now inspect logout handling", "summary":"Check logout"})
StopExternalSubagent({"task_id":"reviewer"})
listExternalSubagentSessions({})
```

`description` and `prompt` are required strings. `subagent_type` is optional and
supports only `general-purpose`; it never selects the execution agent. `model`
is required and names one callable preset; there is no default preset. The preset
fixes agent, model and effort; there is no `effort` parameter and no per-call
override. Reports name the preset, agent, actual model and effort. `isolation` accepts native spellings
`worktree` and `remote` in the schema but both are explicitly rejected by current
adapters, never substituted with local execution. Names start with an ASCII
letter or digit and contain 1–64 ASCII letters, digits, `_`, or `-`.
Deprecated `team_name` and native `mode` values (`acceptEdits`, `auto`,
`bypassPermissions`, `default`, `dontAsk`, `plan`) are accepted and ignored;
they do not grant permissions. Unknown properties, nulls, wrong types, and
unsupported values are rejected.

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

Objects and listExternalSubagentSessions are scoped to the current MCP connection. Reconnecting
loses actionable objects; native transcript files may remain, but AgentX does
not claim cross-connection recovery. This scope also applies when the parent
Claude session resumes. Session loss or identity mismatch is failure, not retry.

## Native execution and permission limits

| Adapter | Boundary | Native continuation | Effort |
| --- | --- | --- | --- |
| Codex | `sandbox_mode="read-only"`, `approval_policy="never"` | Exact thread UUID | Preset effort via `model_reasoning_effort`, or native default; native errors remain errors |
| Pi | `--tools read,grep,find,ls --no-extensions --no-skills --no-prompt-templates --no-approve` | Dedicated exact session path and pinned native ID | Native default only; presets with effort are invalid |

Pi's allowlist is not an OS sandbox. Native authentication/configuration and
project context still apply; these credentials and policies are not inherited
from Claude. AgentX does not widen permissions for deprecated `mode`. Other
agents (including Gemini) remain available for ordinary AgentX launch/exec but
are not delivered MCP combinations in this integration.

Reports identify preset, execution agent, actual selected model, effort, ExternalSubagent/task
IDs, status, failure reason, and continuation/cancellation instructions. Reports
and channels are labelled untrusted ExternalSubagent content and never user
authorization. Available native usage is shown as backend-reported external
usage with native field names; unknown usage stays unknown. Codex resume usage
may include context/cumulative fields; AgentX does not reinterpret it as Claude
usage or fabricate tool counts. `AX_LOG=debug` logs raw command plans, stdin,
stdout, stderr and errors to stderr; it may contain task contents.

## Verification

```sh
make verify
make test-mcp-race
make smoke-mcp-management
make smoke-mcp-contract
make smoke-mcp-live
make smoke-mcp-channel
make smoke-mcp-host-live
```

The live backend suite uses existing native logins, isolated AgentX configuration,
and Codex `gpt-5.6-luna` / Pi `openai-codex/gpt-5.6-luna`. Override
`AX_MCP_CODEX_MODEL` / `AX_MCP_PI_MODEL` with exact accessible models. It verifies
native inference, independent memory continuation and actual cancellation. The
channel suite starts real interactive Claude in a PTY with a deterministic
delayed backend fixture; it requires both channel registration and a subsequent
assistant transcript containing a token absent from its prompt. This separates
host-consumption evidence from actual-backend evidence. Neither fixture tests
nor a server notification alone prove native execution or host delivery.

See [acceptance evidence](subagents-acceptance.md) for scenarios and limitations.

The four names above replace the earlier MCP `Agent`, `SendMessage`, `TaskStop`
and `ListAgents` names. Old names are rejected; there are no compatibility aliases.
The `subagent_type` role and lifecycle are unchanged; `model` now names a preset
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
