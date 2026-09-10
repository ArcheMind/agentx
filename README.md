# agentx

`agentx` is a native-first runtime manager for AI coding-agent CLIs. Its command is `ax`.

It keeps each agent's executable, configuration, credentials, and sessions native. The shared layer is the workflow: locate or install an agent, launch its native subscription login, read models from verified native sources, select a model at launch, and delegate cross-agent session operations to the packaged CASR integration.

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
ax install casr

# Open the agent's native subscription OAuth flow
ax auth login claude
ax auth login codex
ax auth login gemini
ax auth login opencode
ax auth login pi

# Read available models from verified native sources
ax models codex
ax models opencode
ax models pi
ax --yaml models codex

# Select a model and launch the native agent
ax run codex --model gpt-5.4 --cwd .
ax run claude --model sonnet -- --permission-mode plan

# CASR remains the native source of truth for cross-agent sessions
ax session providers
ax session list --limit 20 --sort date
ax session info <session-id>
ax session resume claude <session-id>
```

Arguments after `--` pass directly to the native agent. `ax` does not create a Profile format or copy credentials.

`--json` and `--yaml` are global output selectors for AgentX-owned structured results. They cover agent and model lists plus install, auth, and run dry-run plans. Output from actual agent, installer, and CASR processes remains native and is not re-encoded.

Authentication is delegated to each agent's native flow. Claude, Codex, and OpenCode expose direct login commands. Gemini and Pi expose login inside their interactive clients, so `ax` opens the client and tells you to use `/auth` or `/login`. AgentX never accepts or stores account credentials.

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
