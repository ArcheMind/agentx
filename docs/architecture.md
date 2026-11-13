# Architecture

SessionMan is a small native runtime shell around existing coding-agent CLIs. The architecture keeps ownership boundaries visible.

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
             |     +-- OpenCode
             |     +-- Pi
             |
             +-- Agent Skills store
             |     +-- canonical .agents/skills directories
             |     +-- Claude symbolic-link projections
             |     +-- local and Git installers
             |
             +-- external command runner
```

## CLI layer

`internal/app` owns the resource-first grammar, argument validation, text and structured output, dry-run behavior, and stable error envelopes. It coordinates capabilities but does not contain agent-specific parsing.

## Unified invocation parameters

`internal/agentparams` owns the logical invocation request, parameter defaults and
validation, and the native CLI/RPC mappings for each agent. CLI commands, presets,
headless continuation, and ExternalSubagents carry the same typed fields through
to this layer: model, effort, mode, user configuration, native flags, working
directory, session, appended instructions, and MCP servers.

```text
CLI / preset / session / MCP input
                 |
          Request + Resolve
                 |
      per-agent CLI / RPC mapping
                 |
         native process / session
```

An explicit call override replaces a preset value. An omitted value preserves
the preset value or, without a preset, the native default. Presets apply their
existing mode default before resolution. Native flags are preserved; a known
flag that conflicts with an explicit logical field is rejected. Unsupported
parameter/transport combinations fail before native execution.

`internal/presets` stores user choices and produces a logical request; it does
not render native arguments. `internal/app` parses common options once and
coordinates execution. Subagent adapters own protocol lifecycles, delivery, and
events; parameter mappings stay in `agentparams`. `internal/runtime` executes
plans and does not interpret agent parameters. Native credentials, configuration
files, and session stores retain their native ownership.

## Agent registry and drivers

`internal/drivers` registers one identity per supported agent. Installation, authentication, model discovery, and launch are separate interfaces because native tools do not expose a symmetrical lifecycle.

This avoids a large adapter filled with optional methods and prevents the CLI from advertising a capability simply because another agent implements it.

## Native sessions

`internal/sessions` reads each agent's local native session format and normalizes only the fields required for discovery, inspection, and bounded handoff. The native file path remains visible as the source.

Resume never writes a source agent's database. SessionMan builds a bounded transcript and invokes the target agent through its launch driver.

## Agent Skills

`internal/skills` discovers and validates standard `SKILL.md` directories. The canonical stores are `~/.agents/skills` for user scope and `.agents/skills` for project scope. Codex and Pi consume those directories directly. Claude receives a relative symbolic-link projection under the equivalent `.claude/skills` directory, preserving one physical source of truth. DeepSeek Harness is reported as unsupported; OpenCode remains outside this actively developed integration.

Installation accepts a local directory or a Git repository plus subpath, validates frontmatter and resource safety before writing, rejects conflicts, and stages the canonical copy before publishing it. Source provenance is stored outside the skill in the canonical store's `.sessionman-origins.yaml`; the skill itself remains the standard Agent Skills format.

## External commands

`internal/runtime` executes native commands. Interactive operations retain native stdin, stdout, stderr, and exit behavior. When debug logging is explicitly enabled, each external call emits its raw command plan, stdout, stderr, error, and exit code as a JSON record.

## Data ownership

| Data | Owner | SessionMan behavior |
| --- | --- | --- |
| Agent executable | Native package manager | Locate or delegate installation |
| Credentials | Native agent | Invoke native login, status, and logout flows |
| Models | Verified native source | Parse or report unsupported |
| Sessions | Native agent | Read and normalize without mutation |
| Skills | User or project `.agents/skills` | Validate, install, discover, and project to Claude |
| Run output | Native agent | Pass through unchanged |
| Dry-run plans | SessionMan | Emit as text, JSON, or YAML |

## Adding an agent capability

A change starts with evidence of a stable native source or command. Add the smallest orthogonal driver, register the capability, update the lifecycle audit, and extend the consistency checks. If agents do not share semantics, keep the feature native instead of forcing it into SessionMan.
