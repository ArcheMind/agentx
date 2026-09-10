package drivers

import (
	"fmt"
	"sort"

	"agentx/internal/runtime"
)

type Registry struct {
	agents map[string]runtime.Agent
}

func NewRegistry() Registry {
	agents := []runtime.Agent{
		nativeAgent("claude", "Claude Code", "claude", UnsupportedModels{Agent: "Claude Code"}, false, NativeAuth{Command: runtime.CommandPlan{Executable: "claude", Args: []string{"auth", "login", "--claudeai"}}}),
		nativeAgent("codex", "Codex CLI", "codex", CodexCacheModels{}, true, NativeAuth{Command: runtime.CommandPlan{Executable: "codex", Args: []string{"login"}}}),
		nativeAgent("gemini", "Gemini CLI", "gemini", UnsupportedModels{Agent: "Gemini CLI"}, false, NativeAuth{Command: runtime.CommandPlan{Executable: "gemini"}, Instruction: "Run /auth in Gemini and select Sign in with Google."}),
		nativeAgent("opencode", "OpenCode", "opencode", CommandModels{Plan: runtime.CommandPlan{Executable: "opencode", Args: []string{"models"}}}, true, NativeAuth{Command: runtime.CommandPlan{Executable: "opencode", Args: []string{"auth", "login"}}}),
		nativeAgent("pi", "Pi Coding Agent", "pi", CommandModels{Plan: runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}}}, true, NativeAuth{Command: runtime.CommandPlan{Executable: "pi"}, Instruction: "Run /login in Pi and select the subscription provider."}),
	}
	items := make(map[string]runtime.Agent, len(agents))
	for _, agent := range agents {
		items[agent.ID] = agent
	}
	return Registry{agents: items}
}

func nativeAgent(id, name, binary string, models runtime.ModelDriver, listsModels bool, auth runtime.AuthDriver) runtime.Agent {
	capabilities := []runtime.Capability{
		runtime.CapabilityLaunch,
		runtime.CapabilityAuthLogin,
		runtime.CapabilityModelSelect,
	}
	if listsModels {
		capabilities = append(capabilities, runtime.CapabilityModelList)
	}
	return runtime.Agent{
		ID: id, Name: name, Binary: binary,
		Launch:       NativeLaunch{Binary: binary, ModelFlag: "--model"},
		Models:       models,
		Auth:         auth,
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
