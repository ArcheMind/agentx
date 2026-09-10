# Contributing to AgentX

Thank you for helping make coding-agent workflows simpler without weakening their native tools.

## Before opening a change

Use an issue for a user-visible behavior change or a new agent capability. Small correctness, documentation, and maintenance fixes can go directly to a pull request.

A capability proposal should include native evidence: the exact CLI help, stable local file, or documented protocol that AgentX would use. AgentX does not infer model catalogs, credential state, or session behavior.

## Development setup

Requirements:

- Go 1.25
- The native agent CLI needed by any live integration you are changing

```bash
git clone https://github.com/ArcheMind/agentx.git
cd agentx
make verify
```

`make verify` formats the Go source, runs static checks and tests, audits protocol consistency, builds `bin/ax`, and exercises smoke commands. It is the required local check before a pull request.

## Design rules

- Preserve native configuration, credential, and session stores as facts.
- Prefer deleting duplicated concepts over adding translation layers.
- Keep drivers orthogonal; do not grow a single adapter with unrelated optional methods.
- Add a shared command only when the underlying agents share a real workflow and semantics.
- Pass native arguments after `--` without reinterpretation.
- Log raw external input, output, and errors only through the opt-in debug protocol.
- Do not add compatibility behavior unless the current design requires it.

See [product principles](docs/product-principles.md) and [architecture](docs/architecture.md) for the rationale.

## Pull requests

- Keep each change focused on one user outcome.
- Update user-facing documentation with behavior changes.
- Add or update tests for implementation changes.
- Record native evidence when a driver or capability changes.
- Confirm `make verify` passes.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
