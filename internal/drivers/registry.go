package drivers

import (
	"fmt"
	"sort"

	"github.com/ArcheMind/agentx/internal/runtime"
)

type Registry struct {
	agents map[string]runtime.Agent
}

func NewRegistry() Registry {
	agents := []runtime.Agent{
		nativeAgent("claude", "Claude Code", "claude", "@anthropic-ai/claude-code", UnsupportedModels{Agent: "Claude Code"}, false, NativeAuth{Command: runtime.CommandPlan{Executable: "claude", Args: []string{"auth", "login", "--claudeai"}}, LogoutCommand: runtime.CommandPlan{Executable: "claude", Args: []string{"auth", "logout"}}, StatusCommand: runtime.CommandPlan{Executable: "claude", Args: []string{"auth", "status", "--json"}}, ParseStatus: parseClaudeAuthStatus}),
		nativeAgent("codex", "Codex CLI", "codex", "@openai/codex", CodexCacheModels{}, true, NativeAuth{Command: runtime.CommandPlan{Executable: "codex", Args: []string{"login"}}, LogoutCommand: runtime.CommandPlan{Executable: "codex", Args: []string{"logout"}}, StatusCommand: runtime.CommandPlan{Executable: "codex", Args: []string{"login", "status"}}, ParseStatus: parseCodexAuthStatus}),
		nativeAgent("gemini", "Gemini CLI", "gemini", "@google/gemini-cli", UnsupportedModels{Agent: "Gemini CLI"}, false, NativeAuth{Command: runtime.CommandPlan{Executable: "gemini"}, Instruction: "Run /auth in Gemini and select Sign in with Google.", LogoutCommand: runtime.CommandPlan{Executable: "gemini"}, LogoutInstruction: "Run /logout in Gemini."}),
		nativeAgent("opencode", "OpenCode", "opencode", "opencode-ai", CommandModels{Plan: runtime.CommandPlan{Executable: "opencode", Args: []string{"models"}}}, true, NativeAuth{Command: runtime.CommandPlan{Executable: "opencode", Args: []string{"auth", "login"}}, LogoutCommand: runtime.CommandPlan{Executable: "opencode", Args: []string{"auth", "logout"}}, StatusCommand: runtime.CommandPlan{Executable: "opencode", Args: []string{"auth", "list"}}, ParseStatus: parseOpenCodeAuthStatus}),
		nativeAgent("pi", "Pi Coding Agent", "pi", "@mariozechner/pi-coding-agent", PiModels{Plan: runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}}, Providers: PiAuthFileProviders{}}, true, ProviderAuth{NativeAuth: NativeAuth{Command: runtime.CommandPlan{Executable: "pi"}, Instruction: "Run /login in Pi and select the subscription provider.", LogoutCommand: runtime.CommandPlan{Executable: "pi"}, LogoutInstruction: "Run /logout in Pi and select the provider."}, Providers: PiAuthFileProviders{}}),
	}
	items := make(map[string]runtime.Agent, len(agents))
	for _, agent := range agents {
		items[agent.ID] = agent
	}
	return Registry{agents: items}
}

func nativeAgent(id, name, binary, npmPackage string, models runtime.ModelDriver, listsModels bool, auth runtime.AuthDriver) runtime.Agent {
	capabilities := []runtime.Capability{
		runtime.CapabilityLaunch,
		runtime.CapabilityAuthLogin,
		runtime.CapabilityModelSelect,
	}
	if listsModels {
		capabilities = append(capabilities, runtime.CapabilityModelList)
	}
	if auth.SupportsStatus() {
		capabilities = append(capabilities, runtime.CapabilityAuthStatus)
	}
	if auth.SupportsLogout() {
		capabilities = append(capabilities, runtime.CapabilityAuthLogout)
	}
	return runtime.Agent{
		ID: id, Name: name, Binary: binary,
		Launch:       NativeLaunch{Binary: binary, ModelFlag: "--model"},
		Models:       models,
		Auth:         auth,
		Install:      NPMPackage{Package: npmPackage},
		Capabilities: capabilities,
	}
}

func (r Registry) Get(id string) (runtime.Agent, error) {
	agent, ok := r.agents[id]
	if !ok {
		return runtime.Agent{}, fmt.Errorf("unknown agent %q", id)
	}
	return agent, nil
}

func (r Registry) All() []runtime.Agent {
	items := make([]runtime.Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		items = append(items, agent)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
