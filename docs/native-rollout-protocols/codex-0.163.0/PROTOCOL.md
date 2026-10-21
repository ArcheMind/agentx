# Codex Rollout Protocol — v0.163.0

Reverse-engineered from codex-cli 0.163.0 (commit `3151954`).

Source files consulted:

- `codex-rs/protocol/src/protocol.rs` — `RolloutItem`, `EventMsg`, `SessionMeta`
- `codex-rs/protocol/src/models.rs` — `ResponseItem`, `ContentItem`
- `codex-rs/app-server-protocol/src/protocol/thread_history.rs` — `ThreadHistoryBuilder`
- `codex-rs/core/src/session/rollout_reconstruction.rs` — model context rebuild
- `codex-rs/thread-store/src/local/read_thread.rs` — session lookup
- `codex-rs/rollout/src/list.rs` — filename-based session discovery

## File location and naming

```
~/.codex/sessions/YYYY/MM/DD/rollout-YYYY-MM-DDThh-mm-ss-<uuid>.jsonl
```

The UUID in the filename MUST be a valid UUID v4. `parse_timestamp_uuid_from_filename`
scans from the right for a `-` whose suffix parses as UUID.

## Session discovery

codex looks up sessions in this order:

1. **SQLite state DB** (`~/.codex/state.db`) — primary index, keyed by thread ID.
2. **Filename scan** — walks `~/.codex/sessions/` recursively, matches UUID in filename.
3. **File search** — fuzzy match on UUID string as a fallback.

agentx does NOT write to the SQLite DB. Discovery relies on path (2).
Each resume MUST use a fresh UUID to avoid conflicts with codex-native sessions
and stale SQLite entries from prior attempts.

## JSONL wire format

Each line is a JSON object with top-level `type` and `payload`:

```jsonc
{"type": "<rollout_item_type>", "timestamp": "<ISO-8601>", "payload": { ... }}
```

`type` determines the `RolloutItem` variant (tagged enum, `snake_case`):

| type | Rust variant | Purpose |
|------|-------------|---------|
| `session_meta` | `SessionMeta` | Session identity, MUST be first line |
| `response_item` | `ResponseItem` | Model context items |
| `event_msg` | `EventMsg` | TUI display events |
| `turn_context` | `TurnContextItem` | Turn metadata (model, cwd) |
| `world_state` | `WorldStateItem` | World state diffs |

## Two parallel systems

The rollout feeds two independent consumers:

### 1. Model context reconstruction (`rollout_reconstruction.rs`)

Reads `ResponseItem` records to rebuild the conversation the model sees on resume.
Recognizes all `ResponseItem` variants:

- `message` (role=user/assistant/developer) with `ContentItem`:
  - `input_text` / `output_text` / `input_image`
- `function_call` (name, arguments as JSON string, call_id)
- `function_call_output` (call_id, output as string or content_items)
- `custom_tool_call` / `custom_tool_call_output`
- `reasoning`

### 2. TUI turn display (`ThreadHistoryBuilder`)

Reads `EventMsg` records to build `Turn > ThreadItem` for scrollable history.
**Ignores `ResponseItem` entirely** except for hook-prompt extraction.

Key events consumed (all nested in `event_msg` payload, tagged `snake_case`):

| Event type | ThreadItem produced | Required fields |
|-----------|--------------------|----|
| `turn_started` | (opens turn) | `turn_id` |
| `user_message` | `UserMessage` | `message` (text) |
| `agent_message` | `AgentMessage` | `message` (text) |
| `turn_complete` | (closes turn) | `turn_id` |

Minimal turn sequence in the rollout (one user+assistant exchange):

```jsonc
// 1. open turn
{"type":"event_msg","timestamp":"T","payload":{"type":"turn_started","turn_id":"<turn-uuid>"}}

// 2. user input (rendered in TUI)
{"type":"event_msg","timestamp":"T","payload":{"type":"user_message","message":"user text here"}}

// 3. assistant response (rendered in TUI)
{"type":"event_msg","timestamp":"T","payload":{"type":"agent_message","message":"assistant text here"}}

// 4. close turn
{"type":"event_msg","timestamp":"T","payload":{"type":"turn_complete","turn_id":"<turn-uuid>"}}
```

## session_meta required fields

```jsonc
{
  "type": "session_meta",
  "timestamp": "<ISO-8601>",
  "payload": {
    "id": "<uuid>",           // thread ID, must match filename UUID
    "session_id": "<uuid>",   // same as id (custom deserializer copies if absent)
    "cwd": "/path/to/cwd",
    "timestamp": "<ISO-8601>",
    "originator": "agentx",   // required non-Option String
    "cli_version": "agentx",  // required non-Option String
    "source": "cli"           // default is Unknown, "cli" for normal sessions
  }
}
```

Missing `originator` or `cli_version` causes serde deserialization failure →
session appears to not exist.

## User turn boundary detection

For model context reconstruction, a user turn boundary is:

```rust
fn is_user_turn_boundary(item: &ResponseItem) -> bool {
    // AgentMessage always counts
    // Message with role="user" counts unless it's a contextual fragment
    //   (hook prompt or standard context injection)
}
```

Normal user `input_text` content is NOT contextual, so it counts as a boundary.

## Minimum viable rollout for cross-agent resume

A file that provides both model context and TUI history:

```
Line 1:  session_meta          ← session identity
Line 2+: For each turn {
           event_msg/turn_started
           response_item/message role=user     ← model context
           event_msg/user_message              ← TUI display
           response_item/message role=assistant ← model context
           event_msg/agent_message             ← TUI display
           (optional: response_item/function_call + function_call_output for tools)
           event_msg/turn_complete
         }
```

The `ResponseItem`s and `EventMsg`s carry redundant text (model vs TUI),
which is by design — they serve different consumers.
