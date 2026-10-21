package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/subagents"
)

func (a App) subagentManagement(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ax sub-agent-management <enable|disable|status|serve> (presets are managed from bare ax)")
	}
	home := a.presetHome()
	if args[0] == "serve" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return (&subagents.Server{Home: home, Cwd: cwd, Runner: a.Runner}).Serve(ctx, a.Stdin, a.Stdout)
	}
	config, err := subagents.LoadConfig(home)
	if err != nil {
		return err
	}
	switch args[0] {
	case "enable":
		if _, _, _, err := subagents.LoadValidated(home); err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		entry, _ := json.Marshal(map[string]any{"type": "stdio", "command": executable, "args": []string{"sub-agent-management", "serve"}})
		configPath := filepath.Join(home, ".claude.json")
		if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
			configPath = filepath.Join(dir, ".claude.json")
		}
		var root struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		data, readErr := os.ReadFile(configPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}
		if readErr == nil {
			if err := json.Unmarshal(data, &root); err != nil {
				return fmt.Errorf("read Claude user configuration: %w", err)
			}
		}
		registered := false
		if existing, ok := root.MCPServers["agentx"]; ok {
			var actual, want any
			if err := json.Unmarshal(existing, &actual); err != nil {
				return err
			}
			json.Unmarshal(entry, &want)
			if !reflect.DeepEqual(actual, want) {
				return fmt.Errorf("Claude user MCP entry agentx already exists with different configuration; no entry changed. Resolve the conflict explicitly")
			}
			registered = true
		}
		plan := runtime.CommandPlan{Executable: "claude", Args: []string{"mcp", "add-json", "--scope", "user", "agentx", string(entry)}}
		if !registered {
			result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{})
			if err != nil {
				return fmt.Errorf("register Claude MCP entry: %w; %s", err, result.Stderr)
			}
		}
		config.Enabled = true
		if err := subagents.SaveConfig(home, config); err != nil {
			return err
		}
		list, problems, err := presets.Load(home)
		if err != nil {
			return err
		}
		if len(list)+len(problems) == 0 {
			fmt.Fprintf(a.Stdout, "No presets in %s yet; run ax and choose + Create preset. ExternalSubagent rejects work until a callable preset exists.\n", presets.Dir(home))
		}
		fmt.Fprintln(a.Stdout, "AgentX enabled; Claude user MCP entry installed. Restart/reconnect Claude for discovery. Start via ax claude for named AgentX channel opt-in, or use claude --dangerously-load-development-channels server:agentx. Existing Claude sessions are not automatically changed. Channels require first-party Claude access and host feature/org approval. Disabled does not mean cancelled; StopExternalSubagent remains available.")
		return nil
	case "disable":
		config.Enabled = false
		if err := subagents.SaveConfig(home, config); err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, "AgentX disabled for new ExternalSubagent and SendExternalSubagentMessage work immediately. Running/previously accepted work can finish and report; use StopExternalSubagent to cancel. MCP stays connected for cancellation and results. Claude native subagents are unaffected.")
		return nil
	case "status":
		config, list, problems, err := subagents.LoadValidated(home)
		if err != nil {
			return err
		}
		options := []map[string]any{}
		for _, o := range list {
			reason := "static requirements satisfied; remote request not verified"
			if err := config.Ready(o); err != nil {
				reason = err.Error()
			}
			entry := map[string]any{"name": o.Name, "agent": o.Agent, "model": o.Model, "effort": presets.EffortLabel(o.Effort), "description": o.Description, "limits": subagents.Limits(o, config.Grants[o.Name]), "status": reason}
			if grant, ok := config.Grants[o.Name]; ok {
				entry["execution"], entry["auth"] = grant.Execution, grant.Auth
			}
			options = append(options, entry)
		}
		invalid := append([]presets.Problem{}, problems...)
		return writeStructured(a.Stdout, map[string]any{
			"enabled": config.Enabled, "configuration": subagents.ConfigPath(home), "presets_directory": presets.Dir(home), "presets": options, "invalid_presets": invalid,
			"connections":      subagents.Connections(home),
			"host_effective":   "connections lists live AgentX MCP processes and whether each host completed initialization. Empty means no active server observed. Channel delivery requires interactive Claude with named opt-in and feature/org approval; initialization alone cannot verify that host-controlled gate. Confirm channel registration in Claude debug log and an actual completion report. Restart/reconnect after enable or preset changes to refresh the initialization preset list; ax claude supplies --dangerously-load-development-channels server:agentx; direct claude needs that flag.",
			"disable_behavior": "blocks new work, does not cancel accepted work; StopExternalSubagent remains available", "session_scope": "current MCP connection only; reconnect loses actionable ExternalSubagent objects; no automatic replay",
		}, a.structuredDefault())
	default:
		return fmt.Errorf("unknown sub-agent-management action %q", args[0])
	}
}

func (a App) presetHome() string {
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		return override
	}
	return a.Hooks.Home
}

func (a App) prepareMCPSubagents(request *runtime.RunRequest, explicit bool) error {
	config, err := subagents.LoadConfig(a.presetHome())
	if err != nil {
		return err
	}
	if !config.Enabled {
		if explicit {
			return fmt.Errorf("ExternalSubagents disabled; enable with ax sub-agent-management enable")
		}
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	entry, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"agentx": map[string]any{"type": "stdio", "command": executable, "args": []string{"sub-agent-management", "serve"}}}})
	for _, arg := range request.PassthroughArgs {
		if strings.HasPrefix(arg, "--dangerously-load-development-channels") {
			return fmt.Errorf("AgentX adds its own named channel opt-in; remove duplicate channel flag")
		}
	}
	request.PassthroughArgs = append([]string{"--mcp-config", string(entry), "--dangerously-load-development-channels", "server:agentx"}, request.PassthroughArgs...)
	return nil
}
