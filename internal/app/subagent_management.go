package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/subagents"
)

func (a App) preset(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return presetUsageError()
	}
	home := a.presetHome()
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: sm [--json|--yaml] preset list")
		}
		return a.presetList(home)
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: sm [--json|--yaml] preset show <name>")
		}
		return a.presetShow(home, args[1])
	default:
		return presetUsageError()
	}
}

func (a App) mcpServe(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return a.serveMCPAt(ctx, cwd)
}

func (a App) agyMCPServe(ctx context.Context) error {
	cwd := os.Getenv("SM_MCP_CWD")
	if !filepath.IsAbs(cwd) {
		return fmt.Errorf("SessionMan's native agy plugin requires a workspace from sm agy; start agy through SessionMan")
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("SessionMan agy workspace %q is not accessible", cwd)
	}
	return a.serveMCPAt(ctx, cwd)
}

func (a App) serveMCPAt(ctx context.Context, cwd string) error {
	server := &subagents.Server{Home: a.presetHome(), Cwd: cwd, Runner: a.Runner}
	if r, ok := a.Runner.(runtime.ExecRunner); ok && r.Debug {
		server.Log = a.Stderr
	}
	return server.Serve(ctx, a.Stdin, a.Stdout)
}

// mcpDaemon is the hidden per-agent daemon entry spawned by the MCP thin shell.
func (a App) mcpDaemon(ctx context.Context, args []string) error {
	var home, agent string
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--home":
			home = args[i+1]
		case "--agent":
			agent = args[i+1]
		default:
			return fmt.Errorf("usage: sm mcp __daemon --home <home> --agent <id>")
		}
	}
	if home == "" || agent == "" {
		return fmt.Errorf("usage: sm mcp __daemon --home <home> --agent <id>")
	}
	daemon := &subagents.Daemon{Home: home, Dir: subagents.AgentDir(home, agent), Runner: a.Runner}
	if r, ok := a.Runner.(runtime.ExecRunner); ok && r.Debug {
		daemon.Log = a.Stderr
	}
	return subagents.RunDaemon(daemon)
}

func presetUsageError() error {
	return fmt.Errorf("usage: sm [--json|--yaml] preset <list|show> [args...]")
}

func presetEntry(o presets.Preset) map[string]any {
	reason := "static requirements satisfied; login and model access verified per execution"
	if err := subagents.Ready(o); err != nil {
		reason = err.Error()
	}
	entry := map[string]any{"name": o.Name, "agent": o.Agent, "model": o.Model, "effort": presets.EffortLabel(o.Effort), "description": o.Description, "status": reason, "limits": subagents.Limits(o)}
	return entry
}

func (a App) presetList(home string) error {
	list, problems, err := presets.Load(home)
	if err != nil {
		return err
	}
	options := []map[string]any{}
	for _, o := range list {
		options = append(options, presetEntry(o))
	}
	invalid := append([]presets.Problem{}, problems...)
	if a.Output != OutputText {
		return writeStructured(a.Stdout, map[string]any{
			"presets_directory": presets.Dir(home),
			"presets":           options,
			"invalid_presets":   invalid,
		}, a.Output)
	}
	for _, o := range options {
		fmt.Fprintf(a.Stdout, "%-20s %-10s %s\n", o["name"], o["agent"], o["status"])
	}
	for _, p := range invalid {
		fmt.Fprintf(a.Stdout, "%-20s %-10s invalid: %s\n", p.Name, "-", p.Err)
	}
	return nil
}

func (a App) presetShow(home string, name string) error {
	list, problems, err := presets.Load(home)
	if err != nil {
		return err
	}
	var found *presets.Preset
	for i := range list {
		if list[i].Name == name {
			found = &list[i]
			break
		}
	}
	if found == nil {
		for _, p := range problems {
			if p.Name == name {
				return fmt.Errorf("preset %s is invalid: %s", name, p.Err)
			}
		}
		return fmt.Errorf("preset %q not found in %s", name, presets.Dir(home))
	}
	result := presetEntry(*found)
	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}
	for _, key := range []string{"name", "agent", "model", "effort", "description", "status", "limits"} {
		if value, ok := result[key]; ok {
			fmt.Fprintf(a.Stdout, "%s: %v\n", key, value)
		}
	}
	return nil
}

func (a App) presetHome() string {
	if override := os.Getenv("SM_SUBAGENT_HOME"); override != "" {
		return override
	}
	return a.Hooks.Home
}

func (a App) injectClaudeAgentX(request *runtime.RunRequest) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	mountAgentX(request, agentparams.MCPServer{Command: executable, Args: []string{"mcp", "serve"}})
	request.Flags = append([]string{"--allowedTools", "mcp__sessionman__StartExternalSubagent", "mcp__sessionman__SendExternalSubagentMessage", "mcp__sessionman__StopExternalSubagent", "mcp__sessionman__ListExternalSubagentSessions"}, request.Flags...)

	return nil
}

func injectPiAgentX(home string, request *runtime.RunRequest) error {
	path, err := subagents.WritePiBridge(home)
	if err != nil {
		return err
	}
	request.Flags = append(request.Flags, "--extension", path)
	return nil
}

func injectCodexAgentX(request *runtime.RunRequest) {
	bin, err := os.Executable()
	if err != nil {
		return
	}
	server := agentparams.MCPServer{Command: bin, Args: []string{"mcp", "serve"}}
	if subHome := os.Getenv("SM_SUBAGENT_HOME"); subHome != "" {
		server.Env = map[string]string{"SM_SUBAGENT_HOME": subHome}
	}
	mountAgentX(request, server)
}

func mountAgentX(request *runtime.RunRequest, server agentparams.MCPServer) {
	servers := make(map[string]agentparams.MCPServer, len(request.MCP.Servers)+1)
	for name, existing := range request.MCP.Servers {
		servers[name] = existing
	}
	servers["sessionman"] = server
	request.MCP.Servers = servers
}
