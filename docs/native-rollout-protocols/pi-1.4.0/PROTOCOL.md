# Pi Session Protocol — v1.4.0

Based on pi-coding-agent 1.4.0 (`@earendil-works/pi-coding-agent`) bundled docs
(`docs/json.md`, `docs/message-types.md`, `docs/session-format.md`) and captured output.

Source: native session files at `~/.pi/agent/sessions/`.

## File location and naming

```
~/.pi/agent/sessions/--<cwd-dashes>--/<ISO-timestamp>_<uuid>.jsonl
```

## JSONL record types

Each line is a JSON object. The `type` field determines the record kind.

### `session` — MUST be first line

```jsonc
{
  "type": "session",
  "version": 3,
  "id": "<uuid>",
  "timestamp": "<ISO-8601>",
  "cwd": "/path/to/workspace"
}
```

### `message` — conversation messages

All messages share outer envelope:

```jsonc
{
  "type": "message",
  "id": "<short-hex>",
  "parentId": "<parent-id or null>",
  "timestamp": "<ISO-8601>",
  "message": { ... }  // role-specific payload
}
```

#### role=system

```jsonc
"message": {
  "role": "system",
  "content": "",                      // string | TextContent[]
  "sections": {"preamble": "...", "tools": "<tools>...</tools>", "cwd": "/project"},
  "toolsAdded": [{"name": "read", "description": "...", "parameters": {}}],
  "toolsRemoved": [{"name": "write"}], // optional
  "timestamp": <unix-ms-int>
}
```

The leading system message declares the initial prompt sections and tools.
Later system messages patch `sections` by name (`null` removes one) and list
`toolsAdded`/`toolsRemoved`; replaying them in order yields the current state.
Sessions without a leading system message get one declared on the first request.

#### role=user

```jsonc
"message": {
  "role": "user",
  "content": [{"type": "text", "text": "..."}],
  "timestamp": <unix-ms-int>
}
```

#### role=assistant

```jsonc
"message": {
  "role": "assistant",
  "content": [ ... ],          // see Content Blocks below
  "api": "openai-codex-responses",   // provider API identifier
  "provider": "openai-codex",        // provider key
  "model": "gpt-5.6-terra",          // model ID
  "stopReason": "toolUse",           // "toolUse" | "endTurn"
  "rawStopReason": "completed",      // raw API stop reason
  "responseId": "resp_...",          // API response ID
  "timestamp": <unix-ms-int>,
  "usage": {                         // REQUIRED — Pi crashes without this
    "input": 0, "output": 0,
    "cacheRead": 0, "cacheWrite": 0,
    "reasoning": 0, "totalTokens": 0,
    "cost": {
      "input": 0, "output": 0,
      "cacheRead": 0, "cacheWrite": 0,
      "total": 0
    }
  }
}
```

#### role=toolResult

```jsonc
"message": {
  "role": "toolResult",
  "toolCallId": "<call-id>",
  "toolName": "<tool-name>",
  "content": [{"type": "text", "text": "..."}],
  "isError": false,
  "timestamp": <unix-ms-int>
}
```

### Content type per role

| role | `content` |
|------|-----------|
| `system` | `string \| TextContent[]` |
| `user` | `string \| (TextContent \| ImageContent)[]` |
| `custom` | `string \| (TextContent \| ImageContent)[]` |
| `assistant` | always array of `text` / `thinking` / `toolCall` |
| `toolResult` | array of `TextContent \| ImageContent` |
| `bashExecution`, `branchSummary`, `compactionSummary` | no `content` field |

`content`, where present, MUST NOT be null: Pi's token estimator calls
`.length` on it. Consumers must tolerate unknown roles.

### Content blocks (inside assistant messages)

| type | fields | notes |
|------|--------|-------|
| `thinking` | `thinking` (string), `thinkingSignature` (optional string) | reasoning trace |
| `text` | `text` (string), `textSignature` (optional string) | visible response |
| `toolCall` | `id` (string), `name` (string), `arguments` (JSON string) | **NOT** `toolCallId`/`toolName`/`input` |

**Critical:** `toolCall` uses `id`/`name`/`arguments`. Pi's `estimateTokens2`
reads `arguments.length` for token counting — if `arguments` is missing
(e.g. using `input` instead), Pi crashes with `Cannot read properties of
undefined (reading 'length')`.

### `model_change`

```jsonc
{"type": "model_change", "id": "...", "parentId": "...", "timestamp": "<ISO-8601>", "provider": "openai", "modelId": "gpt-4o"}
```

The latest entry is the selected model.

### `thinking_level_change`

```jsonc
{"type": "thinking_level_change", "id": "...", "parentId": "...", "timestamp": "<ISO-8601>", "thinkingLevel": "high"}
```

### Optional record types (not required for cross-agent resume)

| type | purpose |
|------|---------|
| `custom_message` | injected context (hooks, dotfiles) |

## Known Pi bugs (no null guards)

Pi crashes (verified on 0.84.4) on undefined for these paths:

1. `usage.input` — missing usage object on assistant message
2. `usage.cost.total` — missing cost sub-object
3. `arguments.length` — toolCall using wrong field name
4. `content.length` — null content array on any message

All must be present with valid defaults in cross-agent resume output.

## stdout stream (`--print --mode json`)

One JSON object per LF-terminated line. A basic run:
`session`, `agent_start`, `turn_start`, `message_start`/`message_end` (system),
`message_start`/`message_end` (user), `message_start`, `message_update`*,
`message_end` (assistant), `turn_end`, `agent_end`, `agent_settled`.

| event | fields | meaning |
|-------|--------|---------|
| `session` | `version`, `id`, `timestamp`, `cwd` | first line; session header |
| `agent_start` | — | low-level run started |
| `agent_end` | `messages`, `willRetry` | one low-level run ended; retry/compaction recovery may continue |
| `agent_settled` | `aborted` | Pi will not continue automatically |
| `turn_start` | — | assistant turn started |
| `turn_end` | `message`, `toolResults` | assistant response and its tool calls finished |
| `message_start` | `message` | message started |
| `message_update` | `usage`, `assistantMessageEvent` | delta-only assistant content update |
| `message_end` | `message` | authoritative final message |
| `tool_execution_start` / `_update` / `_end` | `toolCallId`, `toolName`, `args`, `partialResult` / `result`, `isError`, `durationMs` | tool lifecycle; `tool_execution_end.result` is an object `AgentToolResult { content: (TextContent | ImageContent)[], details }` (`@earendil-works/pi-agent-core/dist/types.d.ts:485-492`, AgentToolResult at `:370-374`) |
| `queue_update` | `steering`, `followUp` | pending queues changed |
| `entry_appended` | `entry` | extension appended a session entry |
| `session_info_changed` | `name` | display name changed |
| `thinking_level_changed` | `level` | thinking level changed |
| `compaction_start` / `compaction_end` | `reason` / `result`, `aborted`, `willRetry`, `errorMessage` | compaction; `compaction_end.result` is `CompactionResult | undefined` (object) (`@earendil-works/pi-coding-agent/dist/core/agent-session.d.ts:73-76`) |
| `auto_retry_start` / `auto_retry_end` | `attempt`, `maxAttempts`, `delayMs`, `errorMessage` / `success`, `finalError` | assistant-turn retry |
| `summarization_retry_*` | `attempt`, `source`, `reason`, ... | compaction/branch-summary retry |

`message_end.message` uses the per-role shapes above; system messages appear
in the stream since pi 0.86.0, `agent_settled.aborted` since 1.1.0.

AgentX depends only on:

- `session` — first line; `id` is the native session id.
- `message_end` — only `role=assistant` content is read (text blocks joined,
  last assistant wins; `provider/model`, `stopReason`, `errorMessage`, `usage`).
  Other roles are ignored without decoding `content`.
- `agent_settled` — completion signal; `aborted=true` is a failure.
  `agent_end` is not final.

Events AgentX does not depend on may carry fields of any shape; decoding must
not fail on them.
