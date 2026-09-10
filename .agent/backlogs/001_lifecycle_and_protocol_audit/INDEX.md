# Lifecycle and Protocol Completeness Audit

Status: completed on 2026-09-10. The evidence, decisions, and supported matrix are recorded in [`docs/lifecycle-and-protocol-audit.md`](../../../docs/lifecycle-and-protocol-audit.md); verified implementation state belongs in `.agent/state`.

## Auth lifecycle matrix

Maintain an explicit per-Agent matrix for `login`, `status`, and `logout`, based only on verified native capabilities.

Current verified state:

- `login` is implemented for Claude, Codex, Gemini, OpenCode, and Pi through each Agent's native OAuth flow.
- `status` is implemented for Claude, Codex, and OpenCode.
- Gemini and Pi currently report `status` as unsupported because no reliable native Agent-wide status command has been established.
- `logout` is not currently part of the Auth Driver or CLI.
- `AuthStatus` models one Agent-level boolean plus optional method/subscription, while OpenCode and Pi authentication is provider-scoped. `PiModels` already reads Pi's native authenticated-provider list, but `auth status pi` remains unsupported; these two consumers therefore derive conflicting capability conclusions from the same native source.

Completed:

- Verified native `login`, `status`, and `logout` behavior for all five Agents.
- Normalized status as a provider list; Pi status and model filtering share the native provider source.
- Added native logout delegation for all five Agents while preserving Gemini status as explicitly unsupported.

## Package lifecycle product decision

The current product commitment is locate/install. The Package Driver currently plans installation only. Since CASR became built in, `PackageRegistry` merely duplicates the five Agent Registry entries' ID, name, and install association.

Each `PackageDriver` now belongs to its Agent. `PackageRegistry` and the separate Packages help concept were deleted. Package `status`, `update`, and `uninstall` were not added; agent detection already supplies installed status and no additional stable abstraction was established.

## Built-in Session lifecycle audit

The built-in session service currently exposes `providers`, `list`, `info`, and `resume`.

The native session lifecycles were compared and no additional cross-Agent operation was justified. The source dimension is now uniformly `--source`.

## Structured output applicability

Global `--json` and `--yaml` are accepted by real `install`, `run`, and `auth login` commands but have no effect there; only their dry-run plans are AgentX-owned structured results. This makes the top-level help surface broader than the actual format behavior.

JSON/YAML are scoped to AgentX-owned results and dry-run plans. Native passthrough operations reject structured selectors instead of ignoring them.

## Structured error protocol

Global JSON/YAML formatting currently covers successful AgentX-owned structured results; CLI errors remain text.

JSON/YAML now share an error envelope with stable categories and exit codes. Native stdout/stderr are excluded; opt-in debug logs remain a separate protocol.

## Cross-layer consistency audit

`make audit`, included by `make verify`, checks grammar/documentation and capability/driver consistency in both directions.

## CLI grammar audit

The top-level grammar mixes a bare `list` command that actually means Agent listing, an `agents` alias without resource subcommands, and peer verbs/resources such as `which`, `models`, `run`, and `install`.

Resources now lead the hierarchy: `agent`, `auth`, and `session`. Action-leading commands and plural aliases were removed.

## Completion criteria

- Each lifecycle decision is explicit, evidence-backed, and reflected consistently across CLI, capabilities, drivers, docs, and tests.
- Unsupported operations remain explicit rather than being inferred or emulated without a reliable native source.
- Completed features are removed from the open-gap sections or reclassified as verified current state.
