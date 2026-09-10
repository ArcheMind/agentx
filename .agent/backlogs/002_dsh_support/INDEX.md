# DSH / DeepSeek Harness Support

Status: planned; discovery not yet completed. No DSH command, file format, capability, or integration is considered supported until verified by evidence.

## User goal

Add DSH (DeepSeek Harness) support to AgentX while preserving the runtime's native-first, capability-derived design.

## Known AgentX constraints

- The CLI grammar is resource-first: Agent operations live under `ax agent ...`, authentication under `ax auth ...`, and cross-Agent sessions under `ax session ...`.
- Global JSON/YAML selectors apply only to AgentX-owned structured results and dry-run plans. Native process output remains passthrough and is not re-encoded.
- AgentX does not accept, store, proxy, or print credentials.
- AgentX does not invent model catalogs, authentication states, session schemas, or capability claims.
- Drivers are optional and orthogonal. Capabilities are derived from drivers with verified behavior.

## Discovery blockers

The following DSH facts are not yet verified and must be established from the official repository, license, release artifacts, installed binary help/source, or reproducible bare-machine execution before implementation:

- Official repository identity, maintainership, license, supported platforms, release/install mechanism, package ownership, executable name, and version command/output.
- Whether DSH provides native OAuth/subscription login, API-key authentication, or both; the exact `login`, `status`, and `logout` lifecycle; whether auth is account-wide or provider-scoped; native credential location and safe status fields.
- Whether a native model-list source exists; its command or file, schema stability, canonical model identifiers, and whether results must be filtered by authenticated providers.
- Native launch command, model-selection option, working-directory behavior, environment behavior, and native argument passthrough boundary.
- Whether DSH owns a reliable local session store or commands for session list/info/resume; identifiers, source/provider semantics, and safe transcript extraction.
- Which DSH results are machine-readable and which remain native text or interactive output.
- Whether installation belongs on the DSH Agent entry and whether AgentX can produce a deterministic install plan without owning a separate package abstraction.

Unverified concrete commands, flags, paths, JSON/YAML schemas, and session formats are blockers, not provisional supported behavior.

## Intended domain integration

- Add DSH to the Agent Registry only after executable identity and detection/version behavior are verified.
- Attach `Install`, `Auth`, `Models`, `Launch`, and Session integration only where native evidence supports each operation. Omit unsupported drivers and expose unsupported capabilities explicitly.
- Derive advertised capabilities from the implemented drivers and their verified operation-level support.
- Keep installation ownership on the Agent if a stable install plan exists; do not recreate a parallel Package Registry.
- Reuse one provider-auth abstraction for auth status and authenticated-provider model filtering when DSH is provider-scoped.
- Read native model/session sources without fabricating missing data or writing private native stores.
- Preserve native stdout/stderr for real external execution. JSON/YAML may encode only AgentX-owned detections, statuses, lists, errors, and dry-run plans.

## Target CLI shape

Subject to discovery evidence, DSH should fit the existing resource-first surface rather than add top-level special cases:

- `ax agent list` / `ax agent which dsh`
- `ax agent install dsh [--dry-run]`
- `ax agent models dsh`
- `ax agent run dsh [--model ...] [-- native-args...]`
- `ax auth <login|status|logout> dsh` only for verified native auth operations
- `ax session ... --source dsh` only if a reliable native session source exists

These are integration targets, not claims that DSH currently supports the underlying operations.

## Delivery phases

1. **Evidence baseline:** record official provenance, license, installation, executable/version behavior, and the verified capability matrix with raw-command fixtures where safe.
2. **Minimum Agent support:** implement detection, Agent-owned install planning, and native launch with cwd/model/native-argument behavior limited to verified flags.
3. **Auth and models:** implement native auth lifecycle and model listing/filtering only for established sources; share provider authentication state where applicable.
4. **Sessions:** add provider/list/info/resume integration only if a reliable native session source and safe read/resume path exist. Otherwise document Session as unsupported.
5. **Docs, tests, and audit:** synchronize help, README, capabilities, drivers, fixtures, unit/integration/smoke coverage, and consistency audit rules.

## Completion criteria

- Official provenance, license, installation, executable, version, and capability evidence are recorded.
- Help, README, Capability declarations, Driver implementations, and tests describe the same DSH surface.
- Every unsupported operation and known limitation is explicit; no speculative command or format is presented as working.
- AgentX keeps credential custody, native-output, model-source, and session-store boundaries intact.
- `make verify` passes with DSH-specific consistency and behavior coverage.
