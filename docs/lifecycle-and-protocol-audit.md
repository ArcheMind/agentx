# Lifecycle and protocol audit

Verified on 2026-09-10 against the installed native CLIs and their bundled help/source: Claude Code 2.1.206, Codex CLI 0.154.0, DeepSeek Harness 0.1.5-rc.1, OpenCode 0.5.27, and Pi 0.84.4. Antigravity CLI 1.5.8 was added in November 2026 using native help, ACP and SQLite/protobuf evidence. This matrix defines SessionMan's advertised surface; newer native releases must be re-audited before capabilities change.

## DeepSeek Harness

DSH is registered as `dsh` with the official npm package `@deepseek-ai/dsh`. Its installed package metadata identifies the executable as `dsh`, the package as MIT-licensed, and its repository as `deepseek-ai/deepseek-harness`. Native `dsh --help` verifies `--version`, `--preset`, `--patch`, `web`, and `plugin`; the bundled README verifies the `headless`, `acp`, `sdk`, and `sdk-minimal` presets. AgentX supports detection, npm install planning, and raw native launch with working-directory and argument passthrough, so every native preset remains reachable through `ax dsh -- --preset <preset> ...`.

DSH's bundled DeepSeek catalog supplies model discovery. Its local credential provider reports the presence of `DEEPSEEK_API_KEY` in the inherited environment or `$DSH_HOME/.credentials.yaml`; AgentX reports only the provider identity and never prints the value. `ax auth login dsh` starts DSH's native Web UI, whose Models page owns credential configuration. DSH does not expose a model-selection command-line flag, so selection remains native preset configuration rather than a fabricated `ax --model` mapping. Its durable session store is compressed, versioned, preset-configurable, and has no stable standalone list/info/resume command; AgentX does not parse or modify it.

## Authentication

| Agent | Login | Status | Logout | Native evidence |
| --- | --- | --- | --- | --- |
| Claude | supported | supported, single account | supported | `claude auth` lists `login`, `status --json`, and `logout` |
| Codex | supported | supported, single account | supported | `codex login`, `codex login status`, and `codex logout` |
| OpenCode | supported | supported, provider list | supported | `opencode auth` lists `login`, `list`, and `logout` |
| agy | supported, `agy acp --login` | unsupported | supported, interactive `/logout` | Native CLI 1.5.8; authentication stays in the OS keyring; no authoritative native status query |
| Pi | supported, SDK OAuth provider selector | supported, stored-provider list | supported, interactive | Pi SDK `ModelRuntime.login()` writes native `auth.json`; interactive `/logout` remains native |

`AuthStatus.providers` is always a list. A single-account Agent returns zero or one entry; multi-provider Agents return one entry per configured provider. Status never returns credentials. Pi deliberately reports stored native providers only: environment readiness can be checked provider-by-provider with native `pi auth check`, but Pi exposes no stable command that enumerates every possible provider without credentials.

Logout remains native. AgentX launches the direct command where one exists and otherwise opens the native interactive client with the exact slash-command instruction. It does not delete credential files itself.

## Agent installation

AgentX owns only locate and install planning. Each Agent registry entry owns its native installer. agy uses its official platform installer rather than npm, and rejects version pinning because the native installer has no version-selection interface. There is no separate package resource or package registry because it duplicated Agent identity without adding lifecycle semantics.

`status`, `update`, and `uninstall` are not added. Installed status is already part of `agent list`; update and uninstall remain visibly owned by the native package manager until AgentX has a stable requirement beyond forwarding npm commands.

## Sessions

The cross-Agent service supports `list`, `show`, `resume`, `export`, `import` and `continue`. Native session lifecycles are not uniform: Codex advertises resume/fork/archive/unarchive/delete. AgentX does not normalize destructive operations that lack a shared workflow or semantics. The source dimension is consistently named `--source` on session source filters.

## CLI grammar

Resources lead the hierarchy:

- `agent list|show|install|models|run|exec`
- `auth list|show|login|logout`
- `session list|show|resume|export|import|continue`

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

## Antigravity CLI protocol boundary

Ordinary launch uses the native `agy` command with `--model` and native
arguments after `--`. Resume uses `--conversation <id>`. Models come from the
account's `agy models` output; SessionMan does not infer a catalog or account
readiness from keyring contents.

The ExternalSubagent adapter uses ACP v1 over `agy acp --stdio`, including
`session/new`, `session/load`, `session/set_model`,
`session/set_config_option` and `session/prompt`. Provider effort is unverified
when only native `thought_level` acceptance is available. A permanent ACP
migration of Claude, Codex or Pi is outside this integration.

Native session import/export uses SQLite with strict Step protobuf parsing,
new native IDs and a summary-index entry. The unified protocol is unchanged.
See [transcript storage](unified-transcript-format.md#antigravity-cli-native-storage)
and the [acceptance matrix](agy-acceptance.md).
