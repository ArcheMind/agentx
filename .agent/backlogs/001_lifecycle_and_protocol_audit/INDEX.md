# Lifecycle and Protocol Completeness Audit

Status: future plan. This backlog records decisions and gaps to evaluate; it is not current implementation ground truth.

## Auth lifecycle matrix

Maintain an explicit per-Agent matrix for `login`, `status`, and `logout`, based only on verified native capabilities.

Current verified state:

- `login` is implemented for Claude, Codex, Gemini, OpenCode, and Pi through each Agent's native OAuth flow.
- `status` is implemented for Claude, Codex, and OpenCode.
- Gemini and Pi currently report `status` as unsupported because no reliable native Agent-wide status command has been established.
- `logout` is not currently part of the Auth Driver or CLI.

Future work:

- Verify native `status` and `logout` capabilities for every Agent, including their output stability and suitability for non-interactive use.
- Add only capabilities supported by authoritative native behavior; preserve explicit unsupported results otherwise.
- Keep capability declarations, structured results, docs, and tests synchronized with the matrix.

## Package lifecycle product decision

The current product commitment is locate/install. The Package Driver currently plans installation only.

Evaluate whether AgentX should add package `status`, `update`, and `uninstall`. For each operation, decide whether AgentX owns a stable abstraction or delegates visibly to the native package manager/installer. Do not add lifecycle methods merely for interface symmetry.

## Built-in Session lifecycle audit

The built-in session service currently exposes `providers`, `list`, `info`, and `resume`.

Compare those operations with the actual upstream CASR lifecycle and current user workflows. Record whether any additional operation is necessary, and distinguish upstream CASR behavior from AgentX's built-in implementation rather than assuming feature parity by name.

## Structured error protocol

Global JSON/YAML formatting currently covers successful AgentX-owned structured results; CLI errors remain text.

Evaluate a shared structured error envelope for JSON and YAML, including usage errors, unsupported capabilities, external-command failures, exit-code behavior, and protection against leaking credentials or raw sensitive output. External interactive process output and JSON debug logs remain separate protocols.

## Cross-layer consistency audit

Establish a repeatable audit across:

- CLI commands and help text
- advertised capabilities
- Driver interfaces and concrete implementations
- README and other user-facing documentation
- unit, integration, and smoke tests

The audit should flag both directions of drift: implemented behavior missing from documentation/capabilities, and advertised behavior lacking implementation or tests.

## Completion criteria

- Each lifecycle decision is explicit, evidence-backed, and reflected consistently across CLI, capabilities, drivers, docs, and tests.
- Unsupported operations remain explicit rather than being inferred or emulated without a reliable native source.
- Completed features are removed from the open-gap sections or reclassified as verified current state.
