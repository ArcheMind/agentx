# agentx

[![CI](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml/badge.svg)](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml)
[![CI platforms](https://img.shields.io/badge/CI%20platforms-Linux%20%7C%20macOS%20%7C%20Windows-2ea44f)](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ArcheMind/agentx?display_name=tag)](https://github.com/ArcheMind/agentx/releases)
[![License](https://img.shields.io/github/license/ArcheMind/agentx)](LICENSE)

**See every coding agent's work in one place, continue any of it in any agent, and configure them all once.**

```console
$ ax                # every recent session, every agent
$ ax codex          # start a new Codex session
```

Claude Code, Codex CLI, DeepSeek Harness, Gemini CLI, OpenCode, and Pi run through their own executables, configuration, credentials, and session stores.

## One list of every agent's work

`ax` with no arguments opens one screen with your presets and the recent sessions of every agent, grouped by project.

Start from a preset or resume recent work:

```text
New session · Presets
› codex-review  Codex · gpt-5.6-luna · low     Read-only research and code review using native …
  pi-review     Pi · openai-codex/gpt-5.6-luna  Read-only independent research and code review u…
  + Create preset…

Recent sessions · Current workspace
  claude    Sep 11 05:08 (18m)      Design the interactive session-resume experience for the…
  codex     Sep 11 04:42 (47m)      Refactor the OAuth callback and tighten token refresh ha…
  gemini    Sep 11 04:08 (1h 03m)   Map the runtime boundaries before splitting the command …
  opencode  Sep 11 03:50 (1h 10m)   Polish the session list layout and status copy
  pi        Sep 11 03:24 (44m)      Reduce duplicate state in the keyboard navigation handler

Recent sessions · Global
  codex     Sep 10 12:20 (1h 15m)   ../data-pipeline      Fix the streaming response parser …
  claude    Sep 10 10:48 (33m)      ../api-gateway        Review the production timeout inci…
  opencode  Sep 10 10:20 (4h 00m)   ../analytics          Investigate the slow analytics que…
  gemini    Sep 10 09:40 (2h 30m)   ../mobile-app         Prepare the mobile release checkli…
  pi        Sep 10 06:15 (55m)      ../infrastructure     Add a preflight check before the p…

↑/↓ or j/k select · Enter open · m manage presets · q/Esc cancel
```

Non-interactive and filtered:

```console
ax session list --all --limit 20
ax session list --source codex --all --include-subagents
ax session show <session-id> --source claude --peek
```

Child Agent sessions stay hidden unless `--include-subagents` is set.

## Continue it in any agent

Press Enter on a session, then choose who continues it:

```text
Choose an agent:
> claude
  codex
  gemini
  opencode
  pi
```

![Interactive session resume](docs/assets/interactive-resume.gif)

The same hand-off without the screen:

```console
ax session resume codex <claude-session-id> --source claude
```

AgentX converts the session into the target agent's native transcript format and starts its native resume (`claude --resume`, `codex resume`, `pi --session`, …). Source sessions are only read, never modified.

## Call another agent's preset from inside yours

A preset saves agent, model, effort, and when to use it. The same collection feeds `ax` and the MCP tools, so Claude can run your Codex or Pi presets directly:

```console
$ ax preset list
codex-review         codex      static requirements satisfied; login and model access verified per execution
pi-review            pi         static requirements satisfied; login and model access verified per execution

$ claude mcp add --scope user agentx -- "$(command -v ax)" preset serve
```

Then, inside Claude:

> Use my codex-review and pi-review presets to review this change independently,
> then compare their findings.

Each call runs the preset's own native agent in its own session. No relay inference call, no per-task registration. Calls are read-only unless they ask for `write`; `isolation: "worktree"` runs the subagent in a fresh git worktree that is its only writable root.

```text
ExternalSubagent · SendExternalSubagentMessage · StopExternalSubagent · listExternalSubagentSessions
```

Exact inputs, limits, and acceptance evidence: [docs/subagents.md](docs/subagents.md).

## Configure once, every agent follows

```text
~/.agents/skills/<name>/SKILL.md      codex · dsh · gemini · opencode · pi, Claude via .claude/skills
~/.agents/hooks.json                  claude · codex · gemini · pi
~/.agentx/presets/<name>/PRESET.md    ax · the ExternalSubagent tools above
~/.agentx/roles/<name>/ROLE.md        subagent_type for the ExternalSubagent tools
```

```console
$ ax skill install https://github.com/example/agent-skills.git --path skills/review --scope user
$ ax hook list
SCOPE    EVENT             COMMAND
user     SessionStart      echo session started
user     UserPromptSubmit  echo prompt submitted
user     Stop              echo stopped
```

A preset is one small file:

```markdown
---
agent: codex
model: gpt-5.6-luna
effort: low
---

Read-only research and code review. Not for large refactors.
```

`ax` shows `+ Create preset…` on the home screen and `m` edits the highlighted one. Native config stays native: AgentX never merges into your Claude, Codex, Gemini, or Pi configuration; hook and skill bridges are isolated and visible to the agent's own trust review.

## Install

### macOS and Linux

```bash
curl -LsSf https://raw.githubusercontent.com/ArcheMind/agentx/main/install.sh | sh
```

Downloads the release artifact for the current platform, verifies its SHA-256 checksum, and installs `ax` to `$HOME/.local/bin`. Set `AX_INSTALL_DIR` for another directory.

### Go

```bash
go install github.com/ArcheMind/agentx/cmd/ax@latest
```

Prebuilt archives for macOS, Linux, and Windows are on the [releases page](https://github.com/ArcheMind/agentx/releases). See [installation](docs/installation.md) for version pinning, verification, Windows setup, and source builds.

## Supported agents

| Agent | Sessions | Continue in it | Reads `~/.agents/skills` | Portable hooks |
| --- | --- | --- | --- | --- |
| Claude Code | yes | yes | yes, via `.claude/skills` | yes |
| Codex CLI | yes | yes | yes | yes |
| DeepSeek Harness | — | — | yes | — |
| Gemini CLI | yes | yes | yes | yes |
| OpenCode | yes | yes | yes | — |
| Pi | yes | yes | yes | yes |

DeepSeek Harness has no verified session source, so it is not a session or resume target. AgentX can still install, authenticate, and launch it. OpenCode and DeepSeek Harness are not part of the actively developed skill and hook integrations. Exact native versions and evidence: [lifecycle and protocol audit](docs/lifecycle-and-protocol-audit.md).

## Command reference

```text
ax
ax <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
ax [--json|--yaml] list

ax [--json|--yaml] agent <list|show|install|run|exec> [args...]
ax [--json|--yaml] auth <list|show|login|logout> [args...]
ax [--json|--yaml] session <list|show|resume> [args...]
ax [--json|--yaml] skill <list|show|install> [args...]
ax [--json|--yaml] hook list
ax [--json|--yaml] mcp <list|show> [args...]
ax [--json|--yaml] preset <list|show> [args...]
ax [--json|--yaml] convert <to|from> <provider>

ax version
```

`--json` and `--yaml` apply to AgentX-owned data and dry-run plans; native install, login, launch, and resume keep their own interactive output.

```console
ax list                              # readiness, account, and available models per agent
ax agent install codex --dry-run     # then without --dry-run to install
ax auth login codex                  # native authentication
ax agent show codex                  # binary path, capabilities, models
ax skill list                        # skills by filesystem source and reader
ax mcp list                          # MCP servers by filesystem source and reader
ax agent exec codex --model <id> < prompt.txt
ax --json session resume pi <session-id> --source claude --dry-run
```

## Boundaries

AgentX owns the workflow around agents, not the agents:

- no universal agent settings file beyond the documented skills, hooks, presets, and roles;
- no credential storage or proxying;
- no replacement package managers or session databases;
- no invented capabilities or fallbacks when a native source is missing.

Read [architecture](docs/architecture.md) and [product principles](docs/product-principles.md) for the reasoning.

## Development

Go 1.25.

```bash
make verify
./bin/ax version
```

`make verify` is the repository contract: formatting, static checks, tests, protocol audits, build, and smoke checks. Read [CONTRIBUTING.md](CONTRIBUTING.md) before proposing a change. Regenerate the terminal demo with `vhs docs/demo.tape`.

## Security and support

Report vulnerabilities privately per [SECURITY.md](SECURITY.md). Usage questions and confirmed bugs go through [SUPPORT.md](SUPPORT.md).

AgentX is an independent open-source project, not affiliated with Anthropic, OpenAI, Google, OpenCode, or Pi's maintainers. Product names belong to their respective owners.

## License

[MIT](LICENSE) © 2026 ArcheMind
