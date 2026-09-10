# agentx

[![CI](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml/badge.svg)](https://github.com/ArcheMind/agentx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ArcheMind/agentx?display_name=tag)](https://github.com/ArcheMind/agentx/releases)
[![License](https://img.shields.io/github/license/ArcheMind/agentx)](LICENSE)

**One native workflow for every coding agent you already use.**

`agentx` is a native-first runtime manager for AI coding-agent CLIs. Its command is `ax`. It discovers, installs, authenticates, inspects, launches, and resumes Claude Code, Codex CLI, Gemini CLI, OpenCode, and Pi; DeepSeek Harness is supported for its verified discovery, installation, and launch surface. AgentX does not replace their configuration, credentials, or session stores.

```console
$ ax codex --model gpt-5.4
```

Run `ax` without arguments to choose a recent session from the current workspace or every workspace, then choose the target agent and model.

```console
$ ax agent list
claude     /usr/local/bin/claude (2.1.206)
codex      /usr/local/bin/codex (codex-cli 0.154.0)
dsh        not installed
gemini     not installed
opencode   /usr/local/bin/opencode (0.5.27)
pi         /usr/local/bin/pi (0.84.4)

```

## Why agentx

Coding agents are good native tools. The fragmented workflow around them is not.

- **Keep native ownership.** AgentX does not invent a profile format, copy credentials, or rewrite private session databases.
- **Adopt one command at a time.** Use discovery, installation, model selection, launching, or sessions independently.
- **See the plan first.** Mutating operations support `--dry-run`; AgentX-owned results support JSON and YAML.
- **Move between agents.** Inspect five native session formats and resume useful context in a different agent.
- **Fail honestly.** If an agent exposes no verified model or authentication source, AgentX says so instead of inventing data.

The design is inspired by [uv](https://github.com/astral-sh/uv): make the first useful action cheap, preserve established standards, and unify the workflow rather than claiming ownership of every underlying format.

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

## Start in 60 seconds

```bash
# See what is already available
ax agent list

# Preview an installation, then perform it
ax agent install codex --dry-run
ax agent install codex

# Use the agent's native subscription login
ax auth login codex
ax auth status codex

# Inspect verified native models and launch
ax agent models codex
ax agent run codex --model gpt-5.4 --cwd .
```

## Daily shortcuts

Launch any supported agent directly:

```bash
ax codex --model gpt-5.4
ax claude -- --permission-mode plan
```

Run `ax` with no arguments to interactively select either current-workspace or all-workspace sessions, a target agent, and a model. The session is resumed through the same bounded transcript handoff as `ax session resume`.

Arguments after `--` are passed directly to the native agent:

```bash
ax agent run claude --model sonnet -- --permission-mode plan
```

## Cross-agent sessions

Session discovery and transcript normalization are built into `ax`; there is no companion service or database.

```bash
# Sessions default to the current workspace
ax session list

# Inspect a session from any supported native store
ax session list --source codex --all --limit 20 --sort date
ax session info <session-id> --source codex --peek

# Continue its bounded context in another native agent
ax session resume claude <session-id> --source codex
```

AgentX reads native session stores but never modifies them. Resume passes a bounded normalized transcript to the target agent's native interactive command.

## Supported agents

| Agent | Install | Login | Auth status | Model list | Model select | Sessions |
| --- | --- | --- | --- | --- | --- | --- |
| Claude Code | Yes | Yes | Yes | No verified source | Yes | Yes |
| Codex CLI | Yes | Yes | Yes | Native cache | Yes | Yes |
| DeepSeek Harness | Yes | Unsupported | Unsupported | No verified source | Unsupported | Unsupported |
| Gemini CLI | Yes | Interactive | Unsupported | No verified source | Yes | Yes |
| OpenCode | Yes | Yes | Provider list | Native command | Yes | Yes |
| Pi | Yes | Interactive | Provider list | Native command + auth filter | Yes | Yes |

The exact native versions and evidence behind this table live in the [lifecycle and protocol audit](docs/lifecycle-and-protocol-audit.md).

## Command map

```text
ax agent list
ax agent which <agent>
ax agent install <agent> [--version <version>] [--dry-run]
ax agent models <agent>
ax agent run <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]
ax <agent> [--model <model>] [--cwd <path>] [--dry-run] [-- <native args...>]

ax  # interactive session resume: current/all sessions, agent, model

ax auth login <agent> [--dry-run]
ax auth status <agent>
ax auth logout <agent> [--dry-run]

ax session <providers|list|info|resume>
ax session providers
ax session list [--source <provider>] [--workspace <path>|--all]
                [--limit <n>] [--sort date|messages|provider]
ax session info <session-id> [--source <provider>] [--peek|--peek-lines <n>]
ax session resume <target-agent> <session-id> [--source <provider>]
                  [--workspace <path>] [--dry-run]
```

Place `--json` or `--yaml` before the command for AgentX-owned results and dry-run plans. Actual install, authentication, launch, and resume commands retain native interactive output and reject structured mode rather than silently mixing protocols.

```bash
ax --json agent list
ax --yaml session info <session-id> --source codex
ax --json agent run codex --model gpt-5.4 --dry-run
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

## Security and support

Do not report vulnerabilities in public issues; follow [SECURITY.md](SECURITY.md). For usage questions and confirmed bugs, see [SUPPORT.md](SUPPORT.md).

AgentX is an independent open-source project and is not affiliated with Anthropic, OpenAI, Google, OpenCode, or Pi's maintainers. Product names belong to their respective owners.

## License

[MIT](LICENSE) © 2026 ArcheMind
