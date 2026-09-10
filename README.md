# agentx

`agentx` is a native-first runtime manager for AI coding-agent CLIs. Its command is `ax`.

It keeps each agent's executable, configuration, credentials, and sessions native. The shared layer is the workflow: locate or install an agent, launch its native subscription login, read models from verified native sources, select a model at launch, and resume local sessions across agents through AgentX's built-in session service.

## Build

```bash
make verify
./bin/ax version
```

## Commands

```bash
# Locate installed agents and inspect their versions
ax list
ax --json list
ax --yaml list
ax which codex

# Preview or run a native package installation
ax install codex --dry-run
ax install codex --version 0.153.4

# Open the agent's native subscription OAuth flow
ax auth login claude
ax auth login codex
ax auth login gemini
ax auth login opencode
ax auth login pi

# Check native login status (Gemini and Pi report unsupported)
ax auth status claude
ax --json auth status codex
ax --yaml auth status opencode

# Read available models from verified native sources
ax models codex
ax models opencode
ax models pi
ax --yaml models codex

# Select a model and launch the native agent
ax run codex --model gpt-5.4 --cwd .
ax run claude --model sonnet -- --permission-mode plan

# Discover, inspect, and resume native sessions without a separate CASR binary
ax session providers
ax session list --all --limit 20 --sort date
ax --yaml session info <session-id> --source codex
ax session resume claude <session-id> --source codex
```

Arguments after `--` pass directly to the native agent. `ax` does not create a Profile format or copy credentials.

`--json` and `--yaml` are global output selectors for AgentX-owned structured results. They cover agent, model, provider, and session data, authentication status, plus install, auth, run, and session-resume dry-run plans. Output from actual agents and installers remains native and is not re-encoded.

Authentication is delegated to each agent's native flow. Claude, Codex, and OpenCode expose direct login commands and non-interactive status checks. Gemini and Pi expose login inside their interactive clients but no reliable agent-wide status command, so `ax auth status` reports `unsupported` for them. AgentX never accepts, stores, or prints account credentials.

## Sessions

Session discovery and transcript normalization are compiled into `ax`; there is no separate CASR installation or executable. The built-in readers cover the native local stores of Claude Code, Codex, Gemini CLI, OpenCode's JSON session store, and Pi. `session list` is scoped to the current workspace unless `--all` or `--workspace` is supplied.

`session resume` passes a bounded transcript context to the target agent's native interactive command in the selected workspace. It does not modify private provider databases. Context is capped at 120,000 bytes, preserving the beginning and most recent history when truncation is necessary.

## Model sources

Model discovery is capability-based:

- Codex reads its native `~/.codex/models_cache.json`.
- OpenCode runs `opencode models`.
- Pi runs `pi --list-models`.
- Claude Code and Gemini CLI support model selection, but their installed CLIs expose no verified local model-list source. `ax models` reports that limitation instead of returning an invented catalog.

## Debug logging

Set `AX_LOG=debug` or place `--verbose` before the command. Every external command then emits a JSON record containing its raw command input, stdout, stderr, error, and exit code.

```bash
AX_LOG=debug ax list
ax --verbose models opencode
```
