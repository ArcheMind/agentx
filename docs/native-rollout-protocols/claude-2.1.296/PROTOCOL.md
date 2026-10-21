# Claude Code Session Protocol — v2.1.296

Reverse-engineered from Claude Code 2.1.296 (`@anthropic-ai/claude-code`).

Source: native session files at `~/.claude/projects/`.

## File location and naming

```
~/.claude/projects/<encoded-cwd>/<uuid>.jsonl
```

**CWD encoding:** replace all `/` with `-`.
Example: `/Users/agent/work` → `-Users-agent-work`

**Resume token:** the UUID (filename without `.jsonl`), passed as `claude --resume <uuid>`.

Claude Code resolves `--resume <value>` by:
1. Trying `UUID.parse(value)` — if valid, searches for session by UUID
2. Otherwise, treats as session name for fuzzy matching

agentx MUST write the file into Claude's own project directory with a fresh
UUID. Writing to an external path and passing it as `--resume` does NOT work —
Claude never reads arbitrary file paths.

## JSONL record types

Each line is a JSON object. The `type` field determines the record kind.

### Conversation records (required for resume)

All conversation records share an envelope:

```jsonc
{
  "type": "user" | "assistant",
  "sessionId": "<uuid>",
  "cwd": "/path/to/workspace",
  "timestamp": "<ISO-8601>",
  "uuid": "<message-uuid>",
  "parentUuid": "<parent-uuid or empty>",
  "version": "1",
  "userType": "external",
  "isSidechain": false,
  "entrypoint": "cli",
  "message": { ... }
}
```

#### type=user (human input)

```jsonc
"message": {
  "role": "user",
  "content": "plain text"          // string for single text
  // OR array for structured:
  // "content": [{"type": "text", "text": "..."}]
}
```

#### type=user (tool result)

```jsonc
"message": {
  "role": "user",
  "content": [{
    "type": "tool_result",
    "tool_use_id": "<call-id>",
    "content": "output text",
    "is_error": true              // optional, only when true
  }]
}
```

#### type=assistant

```jsonc
"message": {
  "role": "assistant",
  "content": [
    {"type": "thinking", "thinking": "...", "signature": "..."},
    {"type": "text", "text": "..."},
    {"type": "tool_use", "id": "<call-id>", "name": "<tool>", "input": {...}}
  ],
  "model": "claude-opus-4-6",     // optional
  "stop_reason": "end_turn"       // optional
}
```

### Metadata records (optional, may improve resume fidelity)

| type | purpose | key fields |
|------|---------|-----------|
| `last-prompt` | points to latest message | `leafUuid`, `sessionId` |
| `mode` | session mode | `mode`, `sessionId` |
| `permission-mode` | permission level | `permissionMode`, `sessionId` |
| `ai-title` | session title | `aiTitle`, `sessionId` |
| `system` | tool execution events | extensive, includes `toolUseID`, `stopReason` |
| `cost-state` | token/cost tracking | `totalCostUSD`, `modelUsage` |

### Native envelope fields observed but not strictly required

These appear on every native record but may be optional for resume:

| field | example | notes |
|-------|---------|-------|
| `gitBranch` | `"main"` | current git branch |
| `permissionMode` | `"default"` | on user records |
| `promptId` / `promptSource` | UUIDs | prompt tracking |
| `origin` | `"user"` | input origin |
| `requestId` | UUID | on assistant records |
| `session_id` | UUID | duplicate of sessionId on assistant |

## Minimum viable session file

```
Line 1:  type=user    — first user message
Line 2:  type=assistant — first assistant response  
Line 3+: alternating user/assistant/user(tool_result) records
```

No session header record is needed (unlike codex/pi). The `sessionId` on
each record and the filename UUID establish identity.
