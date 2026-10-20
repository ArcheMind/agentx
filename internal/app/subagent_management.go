package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/subagents"
)

func (a App) subagentManagement(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: ax sub-agent-management <configure|enable|disable|status|serve> (configure reads complete JSON config from stdin)")
	}
	home := a.Hooks.Home
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		home = override
	}
	if args[0] == "serve" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return (&subagents.Server{Home: home, Cwd: cwd, Runner: a.Runner}).Serve(ctx, a.Stdin, a.Stdout)
	}
	if args[0] == "configure" {
		data, err := io.ReadAll(io.LimitReader(a.Stdin, 1024*1024+1))
		if err != nil {
			return err
		}
		if len(data) > 1024*1024 {
			return fmt.Errorf("configuration exceeds 1 MiB")
		}
		var config subagents.Config
		if err := subagents.StrictDecode(data, &config); err != nil {
			return err
		}
		if err := subagents.SaveConfig(home, config); err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, "Subagent configuration saved. New work reads current configuration immediately. Reconnect Claude MCP to refresh tool descriptions and options; existing subagent identities are pinned. Enabling requires the enable command to install the Claude MCP entry.")
		return nil
	}
	config, err := subagents.LoadConfig(home)
	if err != nil {
		return err
	}
	switch args[0] {
	case "enable":
		if len(config.Options) == 0 {
			return fmt.Errorf("no combinations configured; run ax sub-agent-management configure first")
		}
		for _, o := range config.Options {
			if o.Enabled {
				if err := o.Ready(); err != nil {
					return fmt.Errorf("option %s: %w", o.Name, err)
				}
			}
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
		fmt.Fprintln(a.Stdout, "AgentX enabled; Claude user MCP entry installed. Restart/reconnect Claude for discovery. Start via ax claude for named AgentX channel opt-in, or use claude --dangerously-load-development-channels server:agentx. Existing Claude sessions are not automatically changed. Channels require first-party Claude access and host feature/org approval. Disabled does not mean cancelled; TaskStop remains available.")
		return nil
	case "disable":
		config.Enabled = false
		if err := subagents.SaveConfig(home, config); err != nil {
			return err
		}
		fmt.Fprintln(a.Stdout, "AgentX disabled for new Agent and SendMessage work immediately. Running/previously accepted work can finish and report; use TaskStop to cancel. MCP stays connected for cancellation and results. Claude native subagents are unaffected.")
		return nil
	case "status":
		options := []map[string]any{}
		for _, o := range config.Options {
			reason := "static requirements satisfied; remote request not verified"
			if err := o.Ready(); err != nil {
				reason = err.Error()
			}
			options = append(options, map[string]any{"name": o.Name, "agent": o.Agent, "model": o.Model, "enabled": o.Enabled, "description": o.Description, "limits": subagents.Limits(o), "status": reason, "efforts": o.Efforts})
		}
		defaultName := config.Default
		if defaultName == "" {
			defaultName = "none; model must be selected"
		}
		return writeStructured(a.Stdout, map[string]any{
			"enabled": config.Enabled, "configuration": subagents.ConfigPath(home), "default": defaultName, "options": options,
			"connections":      subagents.Connections(home),
			"host_effective":   "connections lists live AgentX MCP processes and whether each host completed initialization. Empty means no active server observed. Channel delivery requires interactive Claude with named opt-in and feature/org approval; initialization alone cannot verify that host-controlled gate. Confirm channel registration in Claude debug log and an actual completion report. Restart/reconnect after enable/configure; ax claude supplies --dangerously-load-development-channels server:agentx; direct claude needs that flag.",
			"disable_behavior": "blocks new work, does not cancel accepted work; TaskStop remains available", "session_scope": "current MCP connection only; reconnect loses actionable subagent objects; no automatic replay",
		}, a.structuredDefault())
	default:
		return fmt.Errorf("unknown sub-agent-management action %q", args[0])
	}
}

func (a App) prepareMCPSubagents(request *runtime.RunRequest, explicit bool) error {
	home := a.Hooks.Home
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		home = override
	}
	config, err := subagents.LoadConfig(home)
	if err != nil {
		return err
	}
	if !config.Enabled {
		if explicit {
			return fmt.Errorf("subagents disabled; configure and enable with ax sub-agent-management")
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
