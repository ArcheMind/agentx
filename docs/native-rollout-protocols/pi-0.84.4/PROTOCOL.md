# Pi Session Protocol — v0.84.4

Reverse-engineered from pi-coding-agent 0.84.4 (`@earendil-works/pi-coding-agent`).

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

`content` MUST be a non-null array (even if empty). Pi's token estimator
calls `.length` on it.

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

### Optional record types (not required for cross-agent resume)

| type | purpose |
|------|---------|
| `model_change` | model switch event |
| `thinking_level_change` | reasoning effort change |
| `custom_message` | injected context (hooks, dotfiles) |

## Known Pi bugs (no null guards)

Pi 0.84.4 crashes on undefined for these paths:

1. `usage.input` — missing usage object on assistant message
2. `usage.cost.total` — missing cost sub-object
3. `arguments.length` — toolCall using wrong field name
4. `content.length` — null content array on any message

All must be present with valid defaults in cross-agent resume output.
