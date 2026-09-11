# Troubleshooting

## `ax` is not found after installation

The default installer destination is `$HOME/.local/bin`. Add it to your shell path and open a new terminal:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Persist the line in the startup file for your shell.

## An agent is reported as not installed

`ax agent list` uses the current process `PATH`. Confirm the native executable is visible in the same shell:

```bash
command -v codex
ax agent which codex
```

Use `ax agent install <agent> --dry-run` to inspect the native package installation plan.

## A model list is unsupported

Model selection and model discovery are separate capabilities. Claude Code and Gemini CLI accept model selection but do not expose a verified local catalog in the audited versions. AgentX reports that boundary instead of returning a static list that can become false.

## Authentication status is unsupported

Gemini CLI provides interactive authentication but no reliable agent-wide status source in the audited version. Use its native `/auth` flow.

## No sessions are listed

`ax session list` defaults to the current workspace. Use `--all` to remove workspace filtering, or pass an explicit path:

```bash
ax session list --all
ax session list --workspace /path/to/project
```

Use `ax session providers` to see the native roots AgentX inspects.

Child Agent sessions are hidden from discovery by default. Pass `--include-subagents` to bare `ax` or `ax session list` when you need to inspect them.

## A native command fails

AgentX preserves the native command's exit behavior. Enable the debug protocol to record the raw external command plan and result:

```bash
AX_LOG=debug ax agent list
ax --verbose agent models opencode
```

Debug output may contain sensitive native content. Review and redact it before attaching it to an issue.
