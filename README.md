# agentx

[![CI](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml/badge.svg)](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml)
[![CI platforms](https://img.shields.io/badge/CI%20platforms-Linux%20%7C%20macOS%20%7C%20Windows-2ea44f)](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ArcheMind/agentx?display_name=tag)](https://github.com/ArcheMind/agentx/releases)
[![License](https://img.shields.io/github/license/ArcheMind/agentx)](LICENSE)

**One native workflow for every coding agent you already use.**

Start a coding agent:

```console
$ ax codex
```

Resume recent work:

```console
$ ax
```

`ax` shows recent top-level sessions in one keyboard-navigable selector, with separate `Current workspace` and `Global` sections. Times use your local timezone, and Global entries include their working directory. Move through every session with the arrow keys, press Enter to select one, then choose a target agent and, when supported, a model. AgentX continues through that agent's native CLI. Pass `--include-subagents` to include provider sessions identified as child Agent work.

![Interactive session resume demo](docs/assets/interactive-resume.gif)

`agentx` is a native-first runtime manager for Claude Code, Codex CLI, DeepSeek Harness, Gemini CLI, OpenCode, and Pi. It uses their existing executables, configuration, credentials, and session stores as the source of truth.

## Install

### macOS and Linux

```bash
curl -LsSf https://raw.githubusercontent.com/ArcheMind/agentx/main/install.sh | sh
```

The installer downloads the release artifact for the current platform, verifies its SHA-256 checksum, and installs `ax` to `$HOME/.local/bin`. Set `AX_INSTALL_DIR` to choose another directory.

### Go

```bash
go install github.com/ArcheMind/agentx/cmd/ax@latest
```

Prebuilt archives for macOS, Linux, and Windows are available on the [releases page](https://github.com/ArcheMind/agentx/releases). See [installation](docs/installation.md) for version pinning, verification, Windows setup, and source builds.

## Start

```bash
# Start a new Codex session in the current directory
ax codex

# Or resume recent work interactively
ax
```

Select a model or working directory when starting directly:

```bash
ax codex --model gpt-5.4 --cwd .
```

Arguments after `--` go directly to the native agent:

```bash
ax claude -- --permission-mode plan
ax dsh -- --profile headless "review this change"
ax dsh -- --profile acp
```

## Manage agents

The resource-oriented commands cover setup and inspection without changing the daily launch path.

```bash
# Preview installed agents, authentication, and available models
ax list
ax --yaml list

# Discover native agents already on PATH
ax agent list

# Preview installation, then install through the verified native package
ax agent install codex --dry-run
ax agent install codex

# Use native authentication
ax auth login codex
ax auth status codex

# Inspect models when the agent exposes a verified source
ax agent models codex
ax agent models dsh

# Configure DSH credentials through its native Models page, then inspect status
ax auth login dsh
ax auth status dsh
```

`ax list` is a readiness view. Uninstalled, logged-out, and unknown agents stay on one line; only agents with verified authentication expand their accounts and currently available models. Terminal output uses color for readiness and status, while pipes, redirects, JSON, YAML, and `NO_COLOR` remain free of ANSI sequences.

```text
agents
├── claude
│   ├── account: claude (claude.ai, max)
│   └── models (status unknown)
├── codex (not logged in)
├── gemini (status unknown)
└── pi (not installed)
```

## Explicit session commands

Bare `ax` is the normal resume flow. Use the session commands when you need exact filters, inspection, or a non-interactive target.

```bash
# Sessions default to the current workspace
ax session list

# Inspect a session from any supported native store
ax session list --source codex --all --limit 20 --sort date
ax session list --all --include-subagents
ax session info <session-id> --source codex --peek

# Continue its bounded context in another native agent
ax session resume claude <session-id> --source codex
```

AgentX reads native session stores but never modifies them. Resume passes a bounded normalized transcript to the target agent's native interactive command.

## Why agentx

Coding agents are good native tools. The fragmented workflow around them is not.

- **Make the first action cheap.** Start an agent or recover recent work without navigating a command hierarchy.
- **Keep native ownership.** AgentX does not invent a profile format, copy credentials, or rewrite private session databases.
- **Adopt one command at a time.** Use launching, recovery, discovery, installation, authentication, or model inspection independently.
- **See the plan first.** Mutating operations support `--dry-run`; AgentX-owned results support JSON and YAML.
- **Fail honestly.** If an agent exposes no verified model, authentication, or session source, AgentX says so instead of inventing data.

The design is inspired by [uv](https://github.com/astral-sh/uv): preserve established standards, remove friction from the common path, and expand from immediately useful workflows.

## Supported agents

| Agent | Install | Login | Auth status | Model list | Model select | Sessions |
| --- | --- | --- | --- | --- | --- | --- |
| Claude Code | Yes | Yes | Yes | No verified source | Yes | Yes |
| Codex CLI | Yes | Yes | Yes | Native cache | Yes | Yes |
| DeepSeek Harness | Yes | Web UI | Provider credential | Bundled catalog | Native profile config | Unsupported |
| Gemini CLI | Yes | Interactive | Unsupported | No verified source | Yes | Yes |
| OpenCode | Yes | Yes | Provider list | Native command | Yes | Yes |
| Pi | Yes | Interactive | Provider list | Native command + auth filter | Yes | Yes |

The exact native versions and evidence behind this table live in the [lifecycle and protocol audit](docs/lifecycle-and-protocol-audit.md).

## Command reference

```text
ax
ax <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
ax [--json|--yaml] list

ax agent list
ax agent which <agent>
ax agent install <agent> [--version <version>] [--dry-run]
ax agent models <agent>
ax agent run <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]

ax auth login <agent> [--dry-run]
ax auth status <agent>
ax auth logout <agent> [--dry-run]

ax session <providers|list|info|resume>
ax session providers
ax session list [--source <provider>] [--workspace <path>|--all]
                [--include-subagents] [--limit <n>]
                [--sort date|messages|provider]
ax session info <session-id> [--source <provider>] [--peek|--peek-lines <n>]
ax session resume <target-agent> <session-id> [--source <provider>]
                  [--workspace <path>] [--dry-run]
```

Place `--json` or `--yaml` before the command for AgentX-owned results and dry-run plans. Actual install, authentication, launch, and resume commands retain native interactive output and reject structured mode rather than silently mixing protocols.

```bash
ax --json agent list
ax --yaml list
ax --yaml session info <session-id> --source codex
ax --json codex --model gpt-5.4 --dry-run
```

## Design boundaries

AgentX owns workflow coordination, not the agents themselves. It deliberately does not:

- define a universal user profile or project configuration format;
- store or proxy account credentials;
- replace native package managers or session databases;
- normalize destructive session operations with incompatible semantics;
- fabricate capabilities absent from a verified native source.

Read [architecture](docs/architecture.md) for the component model and [product principles](docs/product-principles.md) for the decisions behind these boundaries.

## Development

AgentX requires Go 1.25.

```bash
make verify
./bin/ax version
```

`make verify` is the repository contract: formatting, static checks, tests, protocol audits, build, and smoke checks. See [CONTRIBUTING.md](CONTRIBUTING.md) before proposing a change.

Regenerate the terminal demo with `vhs docs/demo.tape`.

## Security and support

Do not report vulnerabilities in public issues; follow [SECURITY.md](SECURITY.md). For usage questions and confirmed bugs, see [SUPPORT.md](SUPPORT.md).

AgentX is an independent open-source project and is not affiliated with Anthropic, OpenAI, Google, OpenCode, or Pi's maintainers. Product names belong to their respective owners.

## License

[MIT](LICENSE) © 2026 ArcheMind
