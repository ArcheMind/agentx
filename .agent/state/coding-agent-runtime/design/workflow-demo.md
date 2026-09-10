# Workflow Demo

## Conclusion

The README embeds a reproducible VHS recording of the interactive session-resume workflow. `docs/demo.tape` builds and drives a dedicated demo executable against sanitized session fixtures and a fake command runner; recording the public demo neither reads native user session data nor launches a native coding agent.

## Why

The public demo must demonstrate the real interaction shape while preserving user privacy and native ownership boundaries. Keeping deterministic fixtures and external-process simulation inside the demo harness also makes regeneration independent of a maintainer's installed agents, credentials, and private session stores.

## Files

- `README.md`
- `docs/demo.tape`
- `docs/assets/interactive-resume.gif`
- `docs/demo/main.go`
- `docs/demo/bin/opencode`
- `docs/demo/home/.codex/sessions/2026/09/09/streaming.jsonl`
- `docs/demo/home/.codex/sessions/2026/09/10/auth-flow.jsonl`
