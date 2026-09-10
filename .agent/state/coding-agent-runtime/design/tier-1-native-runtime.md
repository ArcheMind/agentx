# Tier 1: Native-first Runtime Design and Capability Baseline

Status: current design and implementation plan, subordinate to the Tier 0 principles and resources.

## Positioning

Build a native-first runtime shell around existing Agent CLIs. Do not introduce a user-facing configuration standard in the first version.

The unified lifecycle is:

```text
discover -> inspect -> plan -> launch -> locate native session
```

The Agent's native configuration, credential store, and session store remain the source of truth.

## Runtime flow

```text
CLI request
  -> detect native executable, version, auth state, and supported capabilities
  -> resolve a one-shot internal RuntimePlan
  -> launch the native Agent CLI
  -> retain enough metadata to locate and invoke native resume
```

Arguments after `--` pass through to the native CLI so the runtime does not block newly added Agent features.

## Internal IR, not a project standard

`RunRequest` and `RuntimePlan` are ephemeral internal representations. They are not written into a repository and do not require users to migrate native configuration.

```ts
interface RunRequest {
  agent: string
  cwd: string
  requestedModel?: string
  passthroughArgs: string[]
  environmentOverrides: Record<string, string>
}

interface RuntimePlan {
  executable: string
  args: string[]
  cwd: string
  env: Record<string, string>
  nativeConfigPaths: string[]
}
```

## Orthogonal drivers

Do not implement one giant `AgentAdapter` whose optional methods mix unrelated state lifecycles. Split integration by independently supported capability:

```ts
interface LaunchDriver {
  detect(): Detection
  version(): Version
  capabilities(): Capabilities
  planRun(request: RunRequest): RuntimePlan
}

interface AuthDriver {
  status(): AuthStatus
  login(): CommandPlan
  logout(): CommandPlan
}

interface SessionDriver {
  providers(): SessionProvider[]
  list(): SessionDescriptor[]
  info(id: string): SessionDescriptor
  resume(id: string): CommandPlan
}

interface PackageDriver {
  install(version?: string): CommandPlan
}

interface ModelDriver {
  listAvailable(): ModelDescriptor[]
  select(model: string): CommandPlan
}
```

Capability detection should follow from the drivers an Agent actually implements rather than from a separate aspirational matrix. Each Agent owns its install driver; there is no separate package registry. The current product surface supports verified native auth logout but intentionally does not promise package update/uninstall; driver boundaries describe implemented operations, not a requirement for lifecycle symmetry.

## Runtime capability baseline

The following user-set priorities shaped the implemented v0.1.0 capability baseline:

- **P1 — Locate and install:** implement through `PackageDriver` while keeping each Agent's installation mechanism inside its integration.
- **P1 — Cross-Agent Session:** implement session discovery and bounded normalized transcript handoff inside `ax`; invoke native resume commands without writing private Agent databases.
- **P1 — Available models:** read the models an Agent/provider makes available and allow the user to select one through the native invocation.
- **Deferred — Multiple accounts:** integrate AISW where appropriate or use native isolated configuration/credential directories.

These capability rankings no longer define the current work priority. [GitHub repository productization](../requirement/repository-productization-priority.md) superseded them and is now an achieved product-quality baseline. No later work priority is recorded here. The runtime operations `list`, `doctor`, and `run` remain part of the design vocabulary, but no independent implementation priority is assigned to them here.

## Current design boundaries

- A new Profile schema or project configuration standard
- Central credential custody
- MCP or Skills format conversion
- GUI
- A bespoke package ecosystem; `PackageDriver` delegates to the appropriate installation mechanism

These boundaries do not assign priority to capabilities the user has not ranked.
