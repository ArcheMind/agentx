package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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
			return fmt.Errorf("usage: ax [--json|--yaml] preset list")
		}
		return a.presetList(home)
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: ax [--json|--yaml] preset show <name>")
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
	return (&subagents.Server{Home: a.presetHome(), Cwd: cwd, Runner: a.Runner}).Serve(ctx, a.Stdin, a.Stdout)
}

func presetUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] preset <list|show> [args...]")
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
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		return override
	}
	return a.Hooks.Home
}

func (a App) injectClaudeAgentX(request *runtime.RunRequest) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	entry, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"agentx": map[string]any{"type": "stdio", "command": executable, "args": []string{"mcp", "serve"}}}})
	request.PassthroughArgs = append([]string{
		"--mcp-config", string(entry),
		"--allowedTools", "mcp__agentx__StartExternalSubagent", "mcp__agentx__SendExternalSubagentMessage", "mcp__agentx__StopExternalSubagent", "mcp__agentx__ListExternalSubagentSessions",
	}, request.PassthroughArgs...)
	if os.Getenv("AX_CLAUDE_CHANNELS") == "1" {
		request.PassthroughArgs = append([]string{"--channels", "server:agentx", "--dangerously-load-development-channels", "server:agentx"}, request.PassthroughArgs...)
	}
	return nil
}

func injectCodexAgentX(request *runtime.RunRequest) {
	bin, err := os.Executable()
	if err != nil {
		return
	}
	request.PassthroughArgs = append(request.PassthroughArgs,
		"-c", fmt.Sprintf("mcp_servers.agentx.command=%q", bin),
		"-c", `mcp_servers.agentx.args=["mcp","serve"]`,
	)
	if subHome := os.Getenv("AX_SUBAGENT_HOME"); subHome != "" {
		request.PassthroughArgs = append(request.PassthroughArgs,
			"-c", fmt.Sprintf("mcp_servers.agentx.env.AX_SUBAGENT_HOME=%q", subHome),
		)
	}
}
