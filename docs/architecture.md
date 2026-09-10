# Architecture

AgentX is a small native runtime shell around existing coding-agent CLIs. The architecture keeps ownership boundaries visible.

```text
CLI grammar and output protocol
             |
             +-- agent registry
             |     +-- install driver
             |     +-- auth driver
             |     +-- model driver
             |     +-- launch driver
             |
             +-- native session readers
             |     +-- Claude Code
             |     +-- Codex CLI
             |     +-- Gemini CLI
             |     +-- OpenCode
             |     +-- Pi
             |
             +-- external command runner
```

## CLI layer

`internal/app` owns the resource-first grammar, argument validation, text and structured output, dry-run behavior, and stable error envelopes. It coordinates capabilities but does not contain provider-specific parsing.

## Agent registry and drivers

`internal/drivers` registers one identity per supported agent. Installation, authentication, model discovery, and launch are separate interfaces because native tools do not expose a symmetrical lifecycle.

This avoids a large adapter filled with optional methods and prevents the CLI from advertising a capability simply because another provider implements it.

## Native sessions

`internal/sessions` reads each agent's local native session format and normalizes only the fields required for discovery, inspection, and bounded handoff. The native file path remains visible as the source.

Resume never writes a source provider's database. AgentX builds a bounded transcript and invokes the target agent through its launch driver.

## External commands

`internal/runtime` executes native commands. Interactive operations retain native stdin, stdout, stderr, and exit behavior. When debug logging is explicitly enabled, each external call emits its raw command plan, stdout, stderr, error, and exit code as a JSON record.

## Data ownership

| Data | Owner | AgentX behavior |
| --- | --- | --- |
| Agent executable | Native package manager | Locate or delegate installation |
| Credentials | Native agent | Invoke native login, status, and logout flows |
| Models | Verified native source | Parse or report unsupported |
| Sessions | Native agent | Read and normalize without mutation |
| Run output | Native agent | Pass through unchanged |
| Dry-run plans | AgentX | Emit as text, JSON, or YAML |

## Adding an agent capability

A change starts with evidence of a stable native source or command. Add the smallest orthogonal driver, register the capability, update the lifecycle audit, and extend the consistency checks. If providers do not share semantics, keep the feature native instead of forcing it into AgentX.
