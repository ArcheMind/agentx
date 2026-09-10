# Lifecycle and Protocol Completeness Audit

Status: completed on 2026-09-10. The evidence, decisions, and supported matrix are recorded in [`docs/lifecycle-and-protocol-audit.md`](../../../docs/lifecycle-and-protocol-audit.md); verified implementation state belongs in `.agent/state`.

## Auth lifecycle matrix

- Native `login` and `logout` are supported for Claude, Codex, Gemini, OpenCode, and Pi; direct commands or exact interactive native flows are used according to verified capability.
- Native `status` is supported for Claude, Codex, OpenCode, and Pi. Gemini status is explicitly unsupported.
- `AuthStatus` is normalized as a provider list. Pi status and model filtering share the same native provider source.

## Package lifecycle product decision

Each `PackageDriver` belongs to its Agent. The duplicative `PackageRegistry` and separate Packages help concept were deleted. The product commitment remains locate/install: package `status`, `update`, and `uninstall` were not added because agent detection already supplies installed status and no additional stable abstraction was established.

## Built-in Session lifecycle audit

The built-in session service exposes `providers`, `list`, `info`, and `resume`. Native session lifecycles did not justify another shared operation. Source selection is uniformly named `--source`.

## Structured output applicability

JSON/YAML apply to AgentX-owned results and dry-run plans. Native install, login, logout, run, and session-resume execution rejects structured selectors rather than silently ignoring them; native passthrough output is never re-encoded.

## Structured error protocol

JSON/YAML use a shared structured error envelope with stable categories and exit codes. Native stdout/stderr and opt-in JSON debug logs remain separate protocols.

## Cross-layer consistency audit

`make audit`, included by `make verify`, checks grammar/documentation and capability/driver consistency in both directions.

## CLI grammar audit

The CLI grammar is resource-first: `agent`, `auth`, and `session` lead the hierarchy. Action-leading commands and plural aliases were removed.

## Completion criteria

- Each lifecycle decision is explicit, evidence-backed, and reflected consistently across CLI, capabilities, drivers, docs, and tests.
- Unsupported operations remain explicit rather than being inferred or emulated without a reliable native source.
- Completed features are recorded as verified current state without retaining contradictory open-gap descriptions.
