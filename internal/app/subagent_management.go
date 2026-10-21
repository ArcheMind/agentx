package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	case "serve":
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return (&subagents.Server{Home: home, Cwd: cwd, Runner: a.Runner}).Serve(ctx, a.Stdin, a.Stdout)
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

func presetUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] preset <list|show> [args...]")
}

func (a App) presetList(home string) error {
	state, err := subagents.LoadState(home)
	if err != nil {
		return err
	}
	config, list, problems := state.Config, state.Presets, state.Problems
	options := []map[string]any{}
	for _, o := range list {
		reason := "static requirements satisfied; remote request not verified"
		if err := config.Ready(o); err != nil {
			reason = err.Error()
		}
		entry := map[string]any{"name": o.Name, "agent": o.Agent, "model": o.Model, "effort": presets.EffortLabel(o.Effort), "description": o.Description, "limits": subagents.Limits(o, config.Grants[o.Name]), "status": reason}
		if grant, ok := config.Grants[o.Name]; ok && presets.Agents[o.Agent].Subagent {
			entry["execution"], entry["auth"] = grant.Execution, grant.Auth
		}
		options = append(options, entry)
	}
	invalid := append([]presets.Problem{}, problems...)
	if a.Output != OutputText {
		return writeStructured(a.Stdout, map[string]any{
			"presets_directory": presets.Dir(home),
			"presets":          options,
			"invalid_presets":  invalid,
			"grant_problems":   append([]presets.Problem{}, state.GrantProblems...),
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
	state, err := subagents.LoadState(home)
	if err != nil {
		return err
	}
	config := state.Config
	var found *presets.Preset
	for i := range state.Presets {
		if state.Presets[i].Name == name {
			found = &state.Presets[i]
			break
		}
	}
	if found == nil {
		for _, p := range state.Problems {
			if p.Name == name {
				return fmt.Errorf("preset %s is invalid: %s", name, p.Err)
			}
		}
		return fmt.Errorf("preset %q not found in %s", name, presets.Dir(home))
	}
	o := *found
	reason := "static requirements satisfied; remote request not verified"
	if err := config.Ready(o); err != nil {
		reason = err.Error()
	}
	grant := config.Grants[o.Name]
	result := map[string]any{
		"name": o.Name, "agent": o.Agent, "model": o.Model,
		"effort": presets.EffortLabel(o.Effort), "description": o.Description,
		"limits": subagents.Limits(o, grant),
		"status": reason,
	}
	if _, ok := config.Grants[o.Name]; ok && presets.Agents[o.Agent].Subagent {
		result["execution"], result["auth"] = grant.Execution, grant.Auth
	}
	if a.Output != OutputText {
		return writeStructured(a.Stdout, result, a.Output)
	}
	fmt.Fprintf(a.Stdout, "Name: %s\nAgent: %s\nModel: %s\nEffort: %s\nDescription: %s\nStatus: %s\n", o.Name, o.Agent, o.Model, presets.EffortLabel(o.Effort), o.Description, reason)
	if _, ok := config.Grants[o.Name]; ok && presets.Agents[o.Agent].Subagent {
		fmt.Fprintf(a.Stdout, "Execution: %s\nAuth: %s %s (%s)\n", grant.Execution.Policy, grant.Auth.Source, grant.Auth.Path, grant.Auth.Method)
	}
	return nil
}

func (a App) presetHome() string {
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		return override
	}
	return a.Hooks.Home
}

func (a App) prepareMCPSubagents(request *runtime.RunRequest) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	entry, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"agentx": map[string]any{"type": "stdio", "command": executable, "args": []string{"preset", "serve"}}}})
	for _, arg := range request.PassthroughArgs {
		if strings.HasPrefix(arg, "--dangerously-load-development-channels") {
			return fmt.Errorf("AgentX adds its own named channel opt-in; remove duplicate channel flag")
		}
	}
	request.PassthroughArgs = append([]string{"--mcp-config", string(entry), "--dangerously-load-development-channels", "server:agentx"}, request.PassthroughArgs...)
	return nil
}
