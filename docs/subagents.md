# Native external subagents

Start with `ax claude --subagents`. AgentX loads a content-addressed Claude mod
through `--plugin-dir`, without editing user-owned Claude settings. Claude Code
2.1.296 or newer with Mods enabled is required. If Mods are disabled, no external
agent definitions are registered and no Claude-backed substitute is offered.

## What Claude uses

These tools are available directly, without ToolSearch:

```text
mcp__agentx__discover({})
mcp__agentx__discover({"agent":"pi"})
mcp__agentx__define_agent({
  "name":"reviewer",
  "agent":"pi",
  "model":"provider/model",
  "description":"Review changes for correctness",
  "instructions":"Check failure handling and report concrete findings."
})
→ {"subagent_type":"agentx:reviewer", ...}
Agent(subagent_type="agentx:reviewer", prompt="Review the current change")
SendMessage(to=<native agent ID>, message="Check the second concern")
TaskStop(task_id=<native agent ID>)
```

Discovery reuses `ax --json agent list` and `ax --json agent models <agent>`.
It reports installation and subagent execution support separately. Model discovery
is native: unavailable catalogs produce an explicit error, not an invented list.
An omitted model uses the external CLI's default; an explicit ID is passed intact.
The backend reports whether that model is actually available to the account.

Registration creates a native agent type and makes no inference request. Definitions
are scoped to the Claude session and restored on its next start with `--subagents`.
A name cannot be rebound to another configuration; use another role name. Different
roles may be registered concurrently without overwriting one another. Each child
retains its own binding and native external session, even when several children
use the same role. Role instructions are prepended to the first delegated task;
they do not replace the backend's own system prompt.

`agentx:codex` is registered by default. Its model can be set before session creation
with `AX_SUBAGENT_CODEX_MODEL`; custom roles choose their model through the tool.
AgentX supplies no scheduler, voting scheme or fixed agent combination. Claude
chooses the roles, their models, parallelism and follow-up work with native tools.

## Backends and access

| Backend | Execution | Resume | Native read-only setting |
| --- | --- | --- | --- |
| Codex | `exec --json` | `exec resume <UUID>` | `sandbox_mode="read-only"`, `approval_policy="never"` |
| Pi | `--print --mode json` | `--session <UUID>` | `--tools read,grep,find,ls --no-extensions` |
| Gemini | `--output-format stream-json` | `--resume <UUID>` | `--approval-mode plan` |
| Claude | Use native Claude subagents directly | Native | Native |
| OpenCode, DSH | No execution adapter in this bridge | — | — |

OpenCode and DSH remain available through the ordinary AgentX launch interface.
Gemini requires its own workspace trust and plan-mode prerequisites; AgentX does
not override trust. Pi extensions are disabled to keep the advertised built-in
read-only tool set. There is no general sandbox guarantee across the three CLIs:
the table describes their different native policies, not a shared enforcement layer.

Only delegated child user messages are sent. No parent transcript, Claude tool
allowlist, skills or system prompt is translated into backend configuration.
The backend loads its own project instructions and credentials. Claude's native
child working directory is passed to the external process.

## Lifecycle and failure behavior

The mod answers `turn.step` without calling its native model continuation, yielding
text and stop chunks with `usage: null`. There is no intermediary Claude inference
request for the external child. Ordinary parent reasoning and external inference
still consume tokens; external usage is not reported as Claude usage.

The bridge records the external session ID from native events and resumes that
exact ID. It advances the delivered-message cursor when the backend accepts a
turn. Subsequent turns deliver only new user messages. Missing or shortened child
context, changed external session IDs, malformed output, incomplete answers and
provider errors are explicit failures; none silently falls back to Claude.

Native `SendMessage` resumes the same external session. Native cancellation closes
the process iterator and stops AgentX; on Unix AgentX kills the backend process
group as well. Windows descendant-process cleanup remains unverified. Backend tool
traces remain in backend transcripts. An error is returned as an explicit answer,
so Claude's native task row may say completed even though external execution failed.

Persisted state contains role definitions, native child/external session bindings
and delivery cursors. It does not duplicate the native conversation stores. Plugin
projections live under `~/.cache/agentx/subagents`. `AX_LOG=debug` records raw external
commands, input, output and errors through AgentX's runner and the mod debug log.

## Verification

```bash
make verify
make test-subagents
make smoke-subagents
AX_SUBAGENT_SMOKE_MODEL=gpt-5.6-luna make smoke-subagents-live
AX_SUBAGENT_SMOKE_BACKENDS=pi AX_SUBAGENT_SMOKE_MODEL=openai-codex/gpt-5.6-luna make smoke-subagents-live
AX_SUBAGENT_SMOKE_BACKENDS=gemini make smoke-subagents-live
```

Use model IDs available to your own native account. Live smoke incurs external
model usage. The probe replaces parent inference only for the test and rejects
unexpected child inference. All Claude result records must have empty model usage.
Answers must match the expected tokens exactly, so echoed prompts inside diagnostic
errors cannot make a failing backend pass.

The deterministic smoke runs a real Claude engine with fake external executables.
It discovers capabilities, registers three distinct backend roles in parallel,
spawns and resumes each native child, and checks native cancellation plus Unix
external descendant cleanup. Mod tests cover model/role routing, failure behavior,
concurrent definitions, persisted definitions and native discovery.

On 2026-10-13, live Codex and Pi each passed dynamic registration, native creation
and memory-preserving resume with `gpt-5.6-luna` (Pi's ID includes `openai-codex/`).
Both reported zero Claude model usage. Gemini execution was refused by its native
workspace trust check in this checkout; its real successful inference/resume is
not claimed. Its protocol adapter is covered by mod tests and deterministic native
engine smoke.

References: [Claude turn events](https://code.claude.com/docs/en/plugins/mods/events#follow-a-turn),
[version-specific declarations](https://code.claude.com/docs/en/plugins/mods/create#get-the-types-for-your-build),
[Codex non-interactive execution](https://developers.openai.com/codex/noninteractive).
Implementation was also checked against the installed Pi print-mode/session loader
and Gemini headless/session resolver, rather than inferring their protocol from
command names.
