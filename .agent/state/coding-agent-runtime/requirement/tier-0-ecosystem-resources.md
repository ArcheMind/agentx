# Tier 0: 2026 Coding Agent CLI Ecosystem and Resources

## Market structure

The 2026 Coding Agent CLI ecosystem includes Claude Code, OpenAI Codex CLI, Pi Coding Agent, DSH, OpenCode, Gemini CLI, Aider, Amp, Copilot CLI, Goose, Qwen Code, and others.

Existing products divide the lifecycle rather than providing one factual standard runtime:

- Installation and launch managers cover detection, installation, upgrade, repair, uninstall, and launch.
- Runtime/session managers cover processes, workspaces, panes, logs, resume, and collaboration.
- Environment/configuration tools cover runtimes, versions, environment configuration, or profiles without fully modeling Coding Agent lifecycle and capabilities.

The opportunity is therefore not an empty market. It is a fragmented capability surface with no single tool owning the frequent end-to-end runtime path.

## Relevant existing products

- **CLI Launcher** covers installation and launching across many Agent CLIs, including update, uninstall, repair, and dependency checks. Its product shape is primarily an Electron GUI.
- **AISW** already covers material parts of multi-account/Profile management, cross-tool context, workspace binding, doctor workflows, and structured output. Unified accounts and profiles are therefore not greenfield by themselves.
- **CASR — Cross-Agent Session Resumer**: <https://github.com/Dicklesworthstone/cross_agent_session_resumer>. It covers cross-provider session handling through a canonical session IR, native writers, and native resume behavior. Cross-Agent Session conversion is technically established as a separate capability, not a unique primary wedge.

## Candidate capability surface

The broader runtime opportunity spans:

- Agent detection, installation, versions, upgrade, uninstall, and repair
- Authentication-state detection, multiple accounts, and account switching
- Launching and project/user execution context
- Environment variables, default model, and permission modes
- MCP, Skills, and Instructions
- Capability detection and diagnostics
- Session metadata and native resume
- One runtime shared by CLI, GUI, IDE, and automation clients

These items describe the opportunity space and do not assign Tier 1 implementation priorities.

## Constraints from the evidence

- Do not position installation/launch, unified accounts/Profile, doctor, or cross-provider Session conversion as wholly unoccupied categories.
- Preserve native credential stores, OAuth flows, keychains, configuration, and native session formats where possible.
- A universal Session Descriptor may index native sessions, but native resume remains the reliable restoration path.
- A new user-facing Profile schema is not yet justified by the evidence. Common fields should first be validated internally across real adapters.
