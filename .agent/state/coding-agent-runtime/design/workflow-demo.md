# Workflow Demo

## Conclusion

The README embeds a reproducible VHS recording of the grouped interactive session-resume workflow. As of commit `146fda7`, the primary path selects a sanitized Codex source session and hands it to Claude; Claude and OpenCode are both visible target choices. The recording includes the loading and compact two-line Global presentation, uses an explicit neutral light-gray foreground, and records the real selector with width-safe redraw behavior.

`docs/demo.tape` builds and drives a dedicated demo executable against sanitized session fixtures and a fake command runner; recording the public demo neither reads native user session data nor launches a native coding agent.

## Why

The public demo must demonstrate the real interaction shape and cross-Agent value while preserving user privacy and native ownership boundaries. Keeping deterministic fixtures, two fake installed targets, and external-process simulation inside the demo harness makes regeneration independent of a maintainer's installed agents, credentials, and private session stores.

The `146fda7` recording passed multi-frame visual inspection in addition to `make verify`.

## Files

- `README.md`
- `docs/demo.tape`
- `docs/assets/interactive-resume.gif`
- `docs/demo/main.go`
- `docs/demo/bin/claude`
- `docs/demo/bin/opencode`
- `docs/demo/home/.codex/sessions/2026/09/09/streaming.jsonl`
- `docs/demo/home/.codex/sessions/2026/09/10/auth-flow.jsonl`
- `docs/demo/workspace/.gitkeep`
- `docs/demo/other-project/.gitkeep`
