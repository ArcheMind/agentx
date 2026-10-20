# AgentX MCP ExternalSubagents

AgentX exposes `ExternalSubagent`, `SendExternalSubagentMessage`, `StopExternalSubagent`, and `listExternalSubagentSessions` over stdio MCP.
These are explicitly AgentX tools, not official Claude subagents. The selected
native agent executes directly, without a Claude relay inference call. Claude chooses
the combination and any orchestration; AgentX imposes no review/voting workflow.

## Configure and manage

Create a JSON configuration with explicit native model IDs:

```json
{
  "enabled": false,
  "default": "codex-review",
  "options": [
    {
      "name": "codex-review",
      "agent": "codex",
      "model": "gpt-5.6-luna",
      "description": "Research and code review with the native Codex runtime",
      "enabled": true,
      "efforts": ["low", "medium", "high"]
    },
    {
      "name": "pi-review",
      "agent": "pi",
      "model": "openai-codex/gpt-5.6-luna",
      "description": "Independent review using Pi and its read-only tools",
      "enabled": true
    }
  ]
}
```

```sh
ax sub-agent-management configure < combinations.json
ax sub-agent-management enable
ax sub-agent-management status
ax claude
ax sub-agent-management disable
```

Configuration lives at `~/.config/agentx/subagents.json`. `AX_SUBAGENT_HOME`
overrides AgentX's configuration/state root for isolated environments; it does
not change native agent credentials. Exact enabled options and their capability
descriptions, static availability, limits, and efforts appear in the MCP tool
description before selection. Missing executables and invalid configuration are
errors, not confirmed remote availability. Remote authentication and model access
are established only by execution. Codex efforts must match actual model support;
native rejection is returned as failure. Pi efforts are rejected because its
runtime may silently clamp unsupported levels. Omit `default` to require explicit
selection. No native-default model or automatic fallback is used.

`enable` uses Claude's user-scoped MCP configuration command and preserves other
entries. An identical AgentX entry is reused; a conflicting entry is rejected.
Enable/disable state persists across tasks and sessions. `configure` stores the
whole supplied configuration, including `enabled`. Reconnect/refresh MCP after
editing options to refresh the schema; new work reads current configuration.
Existing ExternalSubagents retain their original combination. Changed/removed/disabled
options reject continuation instead of rebinding it. A queued task also checks
that its original option still exists unchanged before native submission.

Disabling takes effect for new `ExternalSubagent` and `SendExternalSubagentMessage` submissions immediately.
Previously accepted work, including queued messages, can finish and deliver
results. `StopExternalSubagent` remains available. Disabling does not affect Claude native
subagents or cancel work. Re-enabling retains combination configuration.

## Claude discovery and background delivery

MCP initialization includes a config-derived preset DATA catalog: configured
names/default, execution agents/models, declared capability descriptions/efforts,
readonly limits, feature enablement and static prerequisites. The same catalog
supplies `ExternalSubagent` metadata and its selectable `model` values. Static
checks do not verify remote availability. `model` selects a preset independently
of the `subagent_type` role; session listing reports created objects, not presets.
Initialization instructions are a connection snapshot: reconnect after editing
configuration to refresh that catalog. A fresh `tools/list` reads current config.

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
selects one configured combination, or uses the explicitly configured default.
`effort` accepts `low`, `medium`, `high`, `xhigh`, `max` at the schema level; set
it only when the user or relevant instructions explicitly request it. Each option
must declare supported levels; unsupported values are rejected. Codex `max` and
all Pi efforts are currently unsupported. `isolation` accepts native spellings
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
| Codex | `sandbox_mode="read-only"`, `approval_policy="never"` | Exact thread UUID | Explicit configured low/medium/high/xhigh; native errors remain errors |
| Pi | `--tools read,grep,find,ls --no-extensions --no-skills --no-prompt-templates --no-approve` | Dedicated exact session path and pinned native ID | Unsupported; requests rejected |

Pi's allowlist is not an OS sandbox. Native authentication/configuration and
project context still apply; these credentials and policies are not inherited
from Claude. AgentX does not widen permissions for deprecated `mode`. Other
agents (including Gemini) remain available for ordinary AgentX launch/exec but
are not delivered MCP combinations in this integration.

Reports identify option, execution agent, actual selected model, ExternalSubagent/task
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
The parameter keys, `model` combination selection, `subagent_type` role,
configuration and lifecycle are unchanged. Reconnect Claude MCP after upgrading
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
