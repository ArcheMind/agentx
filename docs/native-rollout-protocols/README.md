# Native Rollout Protocols

Reverse-engineered session rollout formats for each external agent.

agentx writes these formats when resuming a session into a different agent
(`ax session resume --agent <target>`). The target agent reads its own native
format; agentx must produce files that satisfy both:

1. **Model context** — the target agent loads prior conversation for inference.
2. **TUI display** — the target agent renders scrollable history for the human.

Each subdirectory is named `<agent>-<version>` and contains a single
`PROTOCOL.md` describing the rollout structure at that version. When a new
version changes the format, add a new directory — old ones stay as-is for
diffing.

## Status

| Agent  | Version  | Model context | TUI display | File              |
|--------|----------|:---:|:---:|-----------------------------------|
| codex  | 0.163.0  | ✅  | ✅  | codex-0.163.0/PROTOCOL.md         |
| claude | 2.1.296  | ✅  | ✅  | claude-2.1.296/PROTOCOL.md        |
| pi     | 0.84.4   | ✅  | ⚠️  | pi-0.84.4/PROTOCOL.md             |
| gemini | —        | —   | —   | (placeholder)                     |
