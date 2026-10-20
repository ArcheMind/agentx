# MCP and Skills source inventory

`ax skill list` and `ax mcp list` display Project/User, followed by actual filesystem source paths, their readers, and resource entries. Empty sources, read failures, and unsupported discovery are separate states. One failed source does not hide readable sources.

```
Project

/project/.agents/skills  codex · dsh · gemini · opencode · pi
  alpha

/project/.claude/skills  claude · opencode
  beta

User

/user/.codex/skills  codex
  review
```

This is an illustrative fixture. In a color terminal AX's adopted `.agents/skills` paths appear first and bold within each scope. Agent colors reuse `internal/app/style.go`; pipes and `NO_COLOR` (including an empty value) contain no ANSI. There are no category labels or tree connectors.

`ax {skill|mcp} show <name> [--scope project|user] [--source <source-path>] [--path <resource-path>]` returns source, scope, resource path, applicable readers, and known filesystem projections. Ambiguous names produce explicit selectors. `--path` distinguishes independent resources having the same metadata name in one root. Physical links aggregate within a scope; independent files with equal names or contents remain independent, and Project/User identities remain separate.

Discovery only supplies configuration evidence. It does not probe servers or observe current sessions. Connection and current-session fields remain `unconfirmed` in real discovery results. The shared model can represent a connection with its evidence and a session load with both session ID and evidence; omitting either session field prevents a loaded claim. Fixture evidence is never used as host evidence. Display order does not establish host override precedence, project trust, enabled state, or runtime effectiveness.

## Supported filesystem discovery

| Agent | Skills | MCP |
| --- | --- | --- |
| Claude | User/Project `.claude/skills` | User `.claude.json` (`mcpServers` and only current project's `projects[path].mcpServers`), Project `.mcp.json` |
| Codex | User `CODEX_HOME/skills` and `.system`, User `.agents/skills`; Project `.codex/skills`, ancestor `.agents/skills`/`.codex/skills` through nearest repository boundary | User `CODEX_HOME/config.toml`, Project `.codex/config.toml`, cwd `config.toml`, ancestor `.codex/config.toml` through nearest repository boundary |
| Gemini | User/Project `.gemini/skills` and `.agents/skills` | User/Project `.gemini/settings.json` (`mcpServers`) |
| DSH | User `.dsh/skills` and `.agents/skills`; Project repository-root `.dsh/skills` and `.agents/skills`; direct skill bundles and flat Markdown | Discovery unsupported; this does not assert the host lacks MCP |
| OpenCode | User `.config/opencode/skill(s)`, Project/ancestor `.opencode/skill(s)`, `.claude/skills`, `.agents/skills` | User `.config/opencode/opencode.json(c)`, Project/ancestor `opencode.json(c)` and `.opencode/opencode.json(c)` (`mcp`) |
| Pi | User `.pi/agent/skills`, User `.agents/skills`, Project `.pi/skills`, ancestor `.agents/skills`; flat Markdown in Pi roots | Discovery unsupported; installed Pi documents no native MCP |

User roots respect `CODEX_HOME`, `CLAUDE_CONFIG_DIR` (Skills), `DSH_HOME`, `DSH_AGENTS_HOME`, `PI_CODING_AGENT_DIR`, `XDG_CONFIG_HOME`, and `GEMINI_CLI_HOME`. Gemini's override replaces HOME for both `.gemini` and `.agents`. Project ancestors follow only the current chain and stop at the nearest `.git`; siblings are not scanned. Missing conventional roots are absent, readable empty roots are explicit, broken links and invalid skill bundle metadata are errors.

Custom settings skill paths, plugin/package/bundled runtime sources outside listed roots, managed policies, URLs, temporary CLI inputs, host trust/enable filtering, and running-session state are not comprehensively discovered. Each affected Agent has an explicit unsupported-discovery entry. These entries are capability boundaries, not empty resources. AX has no existing MCP common-store contract; no fictitious store or special emphasis is introduced. The Skill installation support matrix remains a separate installation contract.

## Evidence used for the discovery matrix

Read-only local source/document inspection on 2026-10-20 established the paths; it does not establish that a particular session loaded anything:

- Gemini installed bundle `docs/cli/skills.md:35–55`, `docs/cli/settings.md:11–12`, `docs/tools/mcp-server.md:92–137`; `bundle/chunk-DHQ53XVO.js:243246,244301–244342` establishes HOME override semantics.
- Pi installed `docs/skills.md:24–39`, `docs/environment-variables.md:81`, README no-MCP statement; running bundle's `collectAncestorAgentsSkillDirs` and `userAgentsSkillsDir` establish shared roots and project trust boundaries.
- DSH installed `node_modules/@deepseek-ai/dsh-skill-filesystem/lib/index.js:78,155–177,556–582,799–808` establishes roots, direct bundle/flat shapes and repository boundary. Its MCP plugin discovery is not implemented here.
- Codex local source `/Users/agent/work/codex/codex-rs/core-skills/src/loader.rs:310–415` and `config/src/loader/mod.rs:97–104`, `config_toml.rs:264–265` establish roots and MCP table format. Source evidence is not a claim about installed-version equivalence.
- OpenCode installed binary's configuration discovery and MCP add code establish `.opencode`, global `opencode`, JSON/JSONC `mcp` paths and ancestor lookup. Claude installed 2.1.296 help and implementation establish local/user/project MCP scopes and JSON shape.

TOML uses `github.com/pelletier/go-toml/v2 v2.2.4`, JSONC uses `github.com/tailscale/hujson v0.0.0-20260302212456-ecc657c15afd`; both avoid incomplete custom parsers. Exact module checksums are committed in `go.sum`.

## Directed verification

The repository's existing `make fmt check test audit` entry points passed on the implementation branch. No ad hoc build/test command was substituted. Test results below refer to controlled fixtures, not real host connection/session checks.

| Fixture | Executed test evidence | Result |
| --- | --- | --- |
| F01 | `inventory.TestSourceIdentityAndSelectionF01F02F04F10`, `skills.TestInventoryProjectionIndependentSourcesAndScopesF01F03F04F07F09F10`, `mcp.TestDiscoveryF01F03F04F07F09F10`, `app.TestInventoryTextF01F02F05F06F07` | Pass: both scopes, independent sources and common-path ordering |
| F02 | inventory identity test and app text test | Pass: aggregate readers, alpha once, beta reader suffix |
| F03 | `skills.TestMultipleAliasesAndSameRootNamesF03F09`, `mcp.TestMCPLinkProjectionsF03` | Pass: real fixture symlinks; one resource and all known alias paths/readers |
| F04 | inventory identity test and Skills source/scope test | Pass: same-name same-content independent sources retained |
| F05 | app text test | Pass: rich-render fixture checks shared-path bold only and existing Agent color constants |
| F06 | app text test | Pass: buffer/non-TTY output has no ANSI; presence of empty NO_COLOR disables color detection |
| F07 | Skills/MCP source tests; `mcp.TestInvalidServersDoNotBecomeEmpty` | Pass: empty/failure/unsupported distinct, readable resources retained |
| F08 | `inventory.TestEvidenceF08`, `app.TestInventoryShowEvidenceF08F09` | Pass: discovered-only, connection-only, and session-bound loaded evidence remain distinct |
| F09 | alias/source selection tests and app evidence test | Pass: projections preserved, source/path selectors resolve ambiguity, evidence remains explicit |
| F10 | Skills/MCP source tests | Pass: other project's fixtures excluded, User kept separate |

Additional regressions: `skills.TestFlatAndNestedAgentApplicability`, `skills.TestInventoryNativeFilesystemRoots`, `skills.TestUserOverrides`, `mcp.TestTOMLReadFailure`. Rich rendering is exercised with controlled renderer color mode, not a real interactive terminal. Actual host connections, session-loaded state, and a live-TTY screenshot are not claimed verified.
