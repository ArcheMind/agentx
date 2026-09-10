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
		nativeAgent("claude", "Claude Code", "claude", "@anthropic-ai/claude-code", UnsupportedModels{Agent: "Claude Code"}, false),
		nativeAgent("codex", "Codex CLI", "codex", "@openai/codex", CodexCacheModels{}, true),
		nativeAgent("gemini", "Gemini CLI", "gemini", "@google/gemini-cli", UnsupportedModels{Agent: "Gemini CLI"}, false),
		nativeAgent("opencode", "OpenCode", "opencode", "opencode-ai", CommandModels{Plan: runtime.CommandPlan{Executable: "opencode", Args: []string{"models"}}}, true),
		nativeAgent("pi", "Pi Coding Agent", "pi", "@mariozechner/pi-coding-agent", CommandModels{Plan: runtime.CommandPlan{Executable: "pi", Args: []string{"--list-models"}}}, true),
		{
			ID: "casr", Name: "Cross Agent Session Resumer", Binary: "casr", Package: runtimePackage(CASRPackage{}),
			Capabilities: []runtime.Capability{runtime.CapabilityInstall, runtime.CapabilitySessionList, runtime.CapabilitySessionRead, runtime.CapabilitySessionWrite},
		},
	}
	items := make(map[string]runtime.Agent, len(agents))
	for _, agent := range agents {
		items[agent.ID] = agent
	}
	return Registry{agents: items}
}

func nativeAgent(id, name, binary, packageName string, models runtime.ModelDriver, listsModels bool) runtime.Agent {
	capabilities := []runtime.Capability{
		runtime.CapabilityLaunch,
		runtime.CapabilityInstall,
		runtime.CapabilityModelSelect,
	}
	if listsModels {
		capabilities = append(capabilities, runtime.CapabilityModelList)
	}
	return runtime.Agent{
		ID: id, Name: name, Binary: binary,
		Launch:       NativeLaunch{Binary: binary, ModelFlag: "--model"},
		Package:      runtimePackage(NPMPackage{Package: packageName}),
		Models:       models,
		Capabilities: capabilities,
	}
}

func runtimePackage(driver runtime.PackageDriver) runtime.PackageDriver {
	return driver
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
