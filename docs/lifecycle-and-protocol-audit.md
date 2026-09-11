# Lifecycle and protocol audit

Verified on 2026-09-10 against the installed native CLIs and their bundled help/source: Claude Code 2.1.206, Codex CLI 0.154.0, DeepSeek Harness 0.1.5-rc.1, Gemini CLI 0.51.0, OpenCode 0.5.27, and Pi 0.84.4. This matrix defines AgentX's advertised surface; newer native releases must be re-audited before capabilities change.

## DeepSeek Harness

DSH is registered as `dsh` with the official npm package `@deepseek-ai/dsh`. Its installed package metadata identifies the executable as `dsh`, the package as MIT-licensed, and its repository as `deepseek-ai/deepseek-harness`. Native `dsh --help` verifies `--version`, `--profile`, `--patch`, `web`, and `plugin`; the bundled README verifies the `headless`, `acp`, `sdk`, and `sdk-minimal` profiles. AgentX supports detection, npm install planning, and raw native launch with working-directory and argument passthrough, so every native profile remains reachable through `ax dsh -- --profile <profile> ...`.

DSH's bundled DeepSeek catalog supplies model discovery. Its local credential provider reports the presence of `DEEPSEEK_API_KEY` in the inherited environment or `$DSH_HOME/.credentials.yaml`; AgentX reports only the provider identity and never prints the value. `ax auth login dsh` starts DSH's native Web UI, whose Models page owns credential configuration. DSH does not expose a model-selection command-line flag, so selection remains native profile configuration rather than a fabricated `ax --model` mapping. Its durable session store is compressed, versioned, profile-configurable, and has no stable standalone list/info/resume command; AgentX does not parse or modify it.

## Authentication

| Agent | Login | Status | Logout | Native evidence |
| --- | --- | --- | --- | --- |
| Claude | supported | supported, single account | supported | `claude auth` lists `login`, `status --json`, and `logout` |
| Codex | supported | supported, single account | supported | `codex login`, `codex login status`, and `codex logout` |
| Gemini | supported, interactive | unsupported | supported, interactive | interactive `/auth` and `/logout`; no Agent-wide status command in CLI help |
| OpenCode | supported | supported, provider list | supported | `opencode auth` lists `login`, `list`, and `logout` |
| Pi | supported, SDK OAuth provider selector | supported, stored-provider list | supported, interactive | Pi SDK `ModelRuntime.login()` writes native `auth.json`; interactive `/logout` remains native |

`AuthStatus.providers` is always a list. A single-account Agent returns zero or one entry; multi-provider Agents return one entry per configured provider. Status never returns credentials. Pi deliberately reports stored native providers only: environment readiness can be checked provider-by-provider with native `pi auth check`, but Pi exposes no stable command that enumerates every possible provider without credentials.

Logout remains native. AgentX launches the direct command where one exists and otherwise opens the native interactive client with the exact slash-command instruction. It does not delete credential files itself.

## Agent installation

AgentX owns only locate and install planning. Each Agent registry entry owns its npm install driver. There is no separate package resource or package registry because it duplicated Agent identity without adding lifecycle semantics.

`status`, `update`, and `uninstall` are not added. Installed status is already part of `agent list`; update and uninstall remain visibly owned by the native package manager until AgentX has a stable requirement beyond forwarding npm commands.

## Sessions

The built-in cross-Agent service remains `providers`, `list`, `info`, and `resume`. Native session lifecycles are not uniform: Gemini advertises project-scoped list/resume/delete, while Codex advertises resume/fork/archive/unarchive/delete. AgentX does not normalize destructive operations that lack a shared workflow or semantics. The source dimension is consistently named `--source` on list, info, and resume.

## CLI grammar

Resources lead the hierarchy:

- `agent list|which|install|models|run`
- `auth login|status|logout`
- `session providers|list|info|resume`

The action-leading commands and the plural `agents`/`sessions` aliases are removed. Agent installation is explicitly Agent-scoped.

## Structured protocols

JSON and YAML apply to AgentX-owned results: Agent detection, model and session data, authentication status, version, and dry-run plans. A real install, login, logout, run, or session resume passes through native interactive output. Combining those executions with `--json` or `--yaml` is rejected; native output is never silently re-encoded.

Errors use the same selected format and preserve process exit behavior:

```json
{
  "error": {
    "code": "usage",
    "message": "...",
    "exit_code": 1
  }
}
```

Stable codes are `usage`, `unsupported_capability`, `external_command`, and `operation_failed`. Structured errors exclude captured native stdout and stderr. Raw command input, output, and errors exist only in the separate opt-in debug-log protocol.

## Repeatable consistency check

`make audit` checks the command grammar in help and README, install-driver ownership, and both directions of capability/driver agreement. `make verify` includes this audit along with the repository's formatter, static checks, tests, build, and smoke command.
