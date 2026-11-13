# SessionMan

[![CI](https://github.com/ArcheMind/sessionMan/actions/workflows/ci.yml/badge.svg)](https://github.com/ArcheMind/sessionMan/actions/workflows/ci.yml)
[![CI platforms](https://img.shields.io/badge/CI%20platforms-Linux%20%7C%20macOS%20%7C%20Windows-2ea44f)](https://github.com/ArcheMind/sessionMan/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ArcheMind/sessionMan?display_name=tag)](https://github.com/ArcheMind/sessionMan/releases)
[![License](https://img.shields.io/github/license/ArcheMind/sessionMan)](LICENSE)

**SessionMan (`sm`) maintains reverse-engineered session formats and invocation interfaces of different coding agents. This makes the previously impossible possible.**

Most common use cases:

1. **Load and continue any agent's session in any other agent** — pick up a Claude session in Codex, or vice versa.
2. **Expose an agent to another agent as a sub-agent** — let Claude call Codex as a tool, or Pi call Claude.
3. **Export a session and send it to a friend** — they restore it into their own agent and keep going.

Minimal footprint, near-zero dependencies — easy to integrate.

```console
$ sm                # every recent session, every agent
$ sm codex          # start a new Codex session
```

## One list of every agent's work

`sm` with no arguments opens one screen with your presets and the recent sessions of every agent, grouped by project.

Start from a preset or resume recent work:

```text
New session · Presets
› codex-review  Codex · gpt-5.6-luna · low     Read-only research and code review using native …
  pi-review     Pi · openai-codex/gpt-5.6-luna  Read-only independent research and code review u…
  + Create preset…

Recent sessions · Current workspace
  claude    Sep 11 05:08 (18m)      Design the interactive session-resume experience for the…
  codex     Sep 11 04:42 (47m)      Refactor the OAuth callback and tighten token refresh ha…
  opencode  Sep 11 03:50 (1h 10m)   Polish the session list layout and status copy
  pi        Sep 11 03:24 (44m)      Reduce duplicate state in the keyboard navigation handler

Recent sessions · Global
  codex     Sep 10 12:20 (1h 15m)   ../data-pipeline      Fix the streaming response parser …
  claude    Sep 10 10:48 (33m)      ../api-gateway        Review the production timeout inci…
  opencode  Sep 10 10:20 (4h 00m)   ../analytics          Investigate the slow analytics que…
  pi        Sep 10 06:15 (55m)      ../infrastructure     Add a preflight check before the p…

↑/↓ or j/k select · Enter open · e export session · m manage presets · q/Esc cancel
```

Non-interactive and filtered:

```console
sm session list --all --limit 20
sm session list --source codex --all --include-subagents
sm session show <session-id> --source claude --peek
```

Child Agent sessions stay hidden unless `--include-subagents` is set.

## Continue it in any agent

Press Enter on a session, then choose who continues it:

```text
Choose an agent:
> agy
  claude
  codex
  opencode
  pi
```

![Interactive session resume](docs/assets/interactive-resume.gif)

The same hand-off without the screen:

```console
sm session resume codex <claude-session-id> --source claude
```

SessionMan converts the session into the target agent's native transcript format and starts its native resume (`agy --conversation`, `claude --resume`, `codex resume`, `pi --session`, …). Source sessions are only read, never modified.

## Move it to another machine

Press e on a session in the interactive home to export it as a unified transcript file (`<agent>-<title>-<id>.json`), or stream it:

```console
sm session export <session-id> --source claude > work.json
sm session import codex work.json      # write into codex's native store, print the new session id
sm --resume work.json                  # import and resume interactively in one step
```

## Call another agent's preset from inside yours

A preset saves agent, model, effort, and when to use it. The same collection feeds `sm` and the MCP tools, so an agent launched through `sm` (for example `sm claude` or `sm codex`) gets the ExternalSubagent tools automatically and can run your other presets directly:

```console
$ sm preset list
claude-review        claude     static requirements satisfied; login and model access verified per execution
pi-review            pi         static requirements satisfied; login and model access verified per execution

$ sm codex
> Ask my claude-review preset to review this change.
```

Each call runs the preset's own native agent in its own session. No relay inference call, no per-task registration. SessionMan passes no permission flags by default (`mode: "normal"`), so each agent's own configuration decides what it may do; `mode: "yolo"` bypasses the agent's permission checks and sandbox. `isolation: "worktree"` runs the subagent in a fresh git worktree; it only changes where the agent works.

```text
StartExternalSubagent · SendExternalSubagentMessage · StopExternalSubagent · ListExternalSubagentSessions
```

Exact inputs, limits, and acceptance evidence: [docs/subagents.md](docs/subagents.md).

## Configure once, every agent follows

```text
~/.agents/skills/<name>/SKILL.md      codex · dsh · opencode · pi, Claude via .claude/skills
~/.agents/hooks.json                  claude · codex · pi; agy SessionStart/Stop
~/.sessionman/presets/<name>/PRESET.md  sm · the ExternalSubagent tools above
```

```console
$ sm skill install https://github.com/example/agent-skills.git --path skills/review --scope user
$ sm hook list
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

`sm` shows `+ Create preset…` on the home screen and `m` edits the highlighted one. Native config stays native: SessionMan never merges into your Claude, Codex, or Pi configuration; hook and skill bridges remain visible to native discovery and trust review. For agy, SessionMan owns a marked plugin under `~/.gemini/config/plugins/sessionman`; it preserves unrelated native configuration.

## Install

### macOS and Linux

```bash
curl -fsSL https://sessionman.dev/install | sh
```

Downloads the release artifact for the current platform, verifies its SHA-256 checksum, and installs `sm` to `$HOME/.local/bin`. Set `SM_INSTALL_DIR` for another directory.

### Go

```bash
go install github.com/ArcheMind/agentx/cmd/sm@latest
```

The Go module retains the original `agentx` path; the installed executable is `sm`.

Prebuilt archives for macOS, Linux, and Windows are on the [releases page](https://github.com/ArcheMind/sessionMan/releases). See [installation](docs/installation.md) for version pinning, verification, Windows setup, and source builds.

Interactive bare `sm` checks for a stable update at most once a day and asks before installing it. A successful update exits without starting a session. Set `SM_NO_UPDATE_CHECK=1` to disable the check and prompt. Agent commands, MCP, and non-interactive invocations never check for updates.

## Supported agents

| Agent | Sessions | Continue in it | Reads `~/.agents/skills` | Portable hooks |
| --- | --- | --- | --- | --- |
| Antigravity CLI (`agy`) | yes | yes | project roots; user install projection | `SessionStart`, `Stop` |
| Claude Code | yes | yes | yes, via `.claude/skills` | yes |
| Codex CLI | yes | yes | yes | yes |
| DeepSeek Harness | — | — | yes | — |
| OpenCode | yes | yes | yes | — |
| Pi | yes | yes | yes | yes |

Antigravity CLI 1.5.8 supports native SQLite sessions, model discovery, presets and ACP ExternalSubagents. Its ordinary exec/continue path rejects temporary `--mcp-config` and `--append-system-prompt`; ACP supports session-scoped MCP. Provider effort is unverified. Its owned plugin is also discovered by plain `agy`, whose MCP bridge requires launching through `sm` to bind the workspace. See [agy acceptance and boundaries](docs/agy-acceptance.md).

DeepSeek Harness has no verified session source, so it is not a session or resume target. SessionMan can still install, authenticate, and launch it. OpenCode and DeepSeek Harness are not part of the actively developed skill and hook integrations. Exact native versions and evidence: [lifecycle and protocol audit](docs/lifecycle-and-protocol-audit.md).

## Command reference

```text
sm
sm --resume <transcript-file>
sm <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
sm [--json|--yaml] status

sm [--json|--yaml] agent <list|show|install|run|exec> [args...]
sm [--json|--yaml] auth <list|show|login|logout> [args...]
sm [--json|--yaml] session <list|show|resume|export|import|continue> [args...]
sm [--json|--yaml] skill <list|show|install> [args...]
sm [--json|--yaml] hook list
sm [--json|--yaml] mcp <list|show|serve> [args...]
sm [--json|--yaml] preset <list|show> [args...]

sm version
```

`--json` and `--yaml` apply to SessionMan-owned data and dry-run plans; native install, login, launch, and resume keep their own interactive output.

```console
sm status                            # readiness, account, and available models per agent
sm agent install codex --dry-run     # then without --dry-run to install
sm auth login codex                  # native authentication
sm agent show codex                  # binary path, capabilities, models
sm skill list                        # skills by filesystem source and reader
sm mcp list                          # MCP servers by filesystem source and reader
sm agent exec codex --model <id> < prompt.txt
sm --json session resume pi <session-id> --source claude --dry-run
sm --json session continue claude --mode yolo --mcp-config mcp.json < transcript.json
```

## Boundaries

SessionMan owns the workflow around agents, not the agents:

- no universal agent settings file beyond the documented skills, hooks, and presets;
- no credential storage or proxying;
- no replacement package managers or session databases;
- no invented capabilities or fallbacks when a native source is missing.

Read [architecture](docs/architecture.md) and [product principles](docs/product-principles.md) for the reasoning.

## Development

Go 1.26.0 or later.

```bash
make verify
./bin/sm version
```

`make verify` is the repository contract: formatting, static checks, tests, protocol audits, build, and smoke checks. Read [CONTRIBUTING.md](CONTRIBUTING.md) before proposing a change. Regenerate the terminal demo with `vhs docs/demo.tape`.

## Security and support

Report vulnerabilities privately per [SECURITY.md](SECURITY.md). Usage questions and confirmed bugs go through [SUPPORT.md](SUPPORT.md).

SessionMan is an independent open-source project, not affiliated with Anthropic, OpenAI, Google, OpenCode, or Pi's maintainers. Product names belong to their respective owners.

## License

[MIT](LICENSE) © 2026 ArcheMind
