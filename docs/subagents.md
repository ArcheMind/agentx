# Native subagents through AgentX

`ax claude --subagents` loads an AgentX-owned mod through `--plugin-dir`. The mod
registers `agentx:codex` using Claude's native `$.agent.register`. It registers no
agent when Mods are disabled, avoiding a static Claude-backed substitute.

The integration targets Claude Code 2.1.296 and newer. That build's generated
`claude-code/index.d.ts` verifies the interfaces used here:

- `classic.SubagentStart` supplies the native child ID and working directory.
- `turn.step` carries `agentId` and can yield its own text and stop chunks without
  calling `next`, so the engine creates the subagent but sends no Claude inference
  request for that child.
- `$.session.messages({ agentId })` reads that child's conversation; an unreadable
  child returns a denial, never the main conversation.
- `$.process.spawn` streams the external process; returning its iterator,
  cancellation of the hook, or unloading the mod kills the process.
- `$.store` retains the native child to Codex thread mapping across mod reloads.

Official references: [turn events](https://code.claude.com/docs/en/plugins/mods/events#follow-a-turn),
[version-specific declarations](https://code.claude.com/docs/en/plugins/mods/create#get-the-types-for-your-build),
[Codex non-interactive execution](https://developers.openai.com/codex/noninteractive).

## Execution and state

The mod launches `ax agent exec codex`, which runs `codex exec --json`. A native
`thread.started` event supplies the Codex session UUID; a subsequent delegation to
the same Claude child uses `codex exec resume <uuid>`. Only new user messages from
the child's transcript are sent on subsequent turns. The mapping records the
message cursor once Codex starts a turn, including interrupted or failed turns, so
an already-submitted prompt is not silently submitted again. A shortened native
transcript fails explicitly rather than guessing a new cursor.

The initial implementation executes read-only tasks. It sets Codex's native
`sandbox_mode="read-only"` and `approval_policy="never"` for new and resumed runs.
There is no translation of Claude's tool policies, skills, memory or system prompt
into Codex settings. Codex loads its own project instructions. Only the delegated
user task is forwarded, and no parent conversation is copied.

AgentX does not recreate a task list, scheduler, conversation store, or credentials.
The mod uses native Claude IDs and the native Codex session. The only additional
state is their association and the delivery cursor. The plugin projection is
content-addressed under `~/.cache/agentx/subagents`; it never edits user-owned
Claude configuration. `AX_LOG=debug` logs raw external commands, stdin, output and
errors through the existing AgentX runner.

## Boundaries

- Returned text is the external agent's final message. Codex tool calls remain in
  Codex's transcript and do not appear as native Claude tool calls.
- An external error is delivered as an explicit error answer. It does not turn
  the native task row into a failed state; the native response was delivered.
- Token counters on the Claude row do not represent external provider usage.
- Native cancellation ends the bridge process. On Unix, AgentX cancels the external
  process group as well; Windows descendant-process cleanup is not yet verified.
- Resume requires the original Codex session and Claude mod state. Missing state
  returns an error; it does not create a replacement conversation silently.
- Only Codex and the built-in `agentx:codex` definition are supported in this first
  integration. Custom external subagent definitions and combinations are follow-up
  work, not aliases silently mapped to Codex.

## Verification

```bash
make verify
make test-subagents
make smoke-subagents
make smoke-subagents-live
```

`test-subagents` uses Claude's own mod test runner, with no inference calls.
`smoke-subagents` drives a real Claude engine with a deterministic Codex executable
and checks native spawn, resume, cancellation, and descendant-process cleanup. A probe answers the parent model step and
refuses any unexpected child model request. `smoke-subagents-live` uses the same
probe with real Codex and native authentication; it incurs Codex usage. Both smoke
commands print the retained evidence directory. No credentials are copied.

Validated on 2026-10-13 with Claude Code 2.1.296 and Codex CLI 0.147.0: native
spawn and `SendMessage` resume succeeded with a real `gpt-5.6-luna` session;
the probe's Claude model usage was empty and its reported cost was zero. The
deterministic cancellation case produced a native `killed` task and left neither
external process running. This does not claim that ordinary parent-agent reasoning
is free; the probe replaces the parent's request solely to measure the bridge.
The machine's default `gpt-6-astra` failed its CLI version requirement, and the
repository's older smoke choice `gpt-6-luna` was refused for the account. Both
errors were returned without falling back to Claude. Use a model available in
your own native Codex model list, for example by setting
`AX_SUBAGENT_CODEX_MODEL` for `make smoke-subagents-live`.
