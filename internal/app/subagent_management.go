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
	"github.com/ArcheMind/agentx/internal/roles"
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
	switch args[0] {
	case "enable":
		entry, err := agentxEntry()
		if err != nil {
			return err
		}
		registered, err := mcpRegistration(home)
		if err != nil {
			return err
		}
		if !registered {
			plan := runtime.CommandPlan{Executable: "claude", Args: []string{"mcp", "add-json", "--scope", "user", "agentx", string(entry)}}
			result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{})
			if err != nil {
				return fmt.Errorf("register Claude MCP entry: %w; %s", err, result.Stderr)
			}
		}
		list, problems, err := presets.Load(home)
		if err != nil {
			return err
		}
		if len(list)+len(problems) == 0 {
			fmt.Fprintf(a.Stdout, "No presets in %s yet; run ax and choose + Create preset. ExternalSubagent rejects work until a callable preset exists.\n", presets.Dir(home))
		}
		fmt.Fprintln(a.Stdout, "AgentX enabled: Claude user MCP entry agentx registered. Restart/reconnect Claude for discovery. Start via ax claude for named AgentX channel opt-in, or use claude --dangerously-load-development-channels server:agentx. Existing Claude sessions are not automatically changed. Channels require first-party Claude access and host feature/org approval.")
		return nil
	case "disable":
		registered, err := mcpRegistration(home)
		if err != nil {
			return err
		}
		if registered {
			plan := runtime.CommandPlan{Executable: "claude", Args: []string{"mcp", "remove", "--scope", "user", "agentx"}}
			result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{})
			if err != nil {
				return fmt.Errorf("remove Claude MCP entry: %w; %s", err, result.Stderr)
			}
		}
		fmt.Fprintln(a.Stdout, "AgentX disabled: Claude user MCP entry agentx removed, so new Claude sessions no longer start the AgentX server. Connections already established keep running until their Claude session ends; their ExternalSubagents can finish and report, and StopExternalSubagent remains available there. Claude native subagents are unaffected.")
		return nil
	case "status":
		registered, err := mcpRegistration(home)
		if err != nil {
			return err
		}
		list, problems, err := presets.Load(home)
		if err != nil {
			return err
		}
		options := []map[string]any{}
		for _, o := range list {
			reason := "static requirements satisfied; login and remote request verified per execution"
			if err := subagents.Ready(o); err != nil {
				reason = err.Error()
			}
			options = append(options, map[string]any{"name": o.Name, "agent": o.Agent, "model": o.Model, "effort": presets.EffortLabel(o.Effort), "description": o.Description, "limits": subagents.Limits(o), "status": reason})
		}
		roleList, roleProblems, err := roles.Load(home)
		if err != nil {
			return err
		}
		declared := []map[string]any{{"name": roles.GeneralPurpose, "description": "implicit; no role instructions added"}}
		for _, role := range roleList {
			declared = append(declared, map[string]any{"name": role.Name, "description": role.Description, "path": roles.Path(home, role.Name)})
		}
		return writeStructured(a.Stdout, map[string]any{
			"enabled": registered, "mcp_entry": claudeConfigPath(home), "presets_directory": presets.Dir(home), "presets": options, "invalid_presets": append([]presets.Problem{}, problems...),
			"roles_directory": roles.Dir(home), "roles": declared, "invalid_roles": append([]presets.Problem{}, roleProblems...),
			"role_behavior":    "subagent_type selects a role; its instructions are task framing only and never change agent, model, effort, write scope or permissions. Codex receives them as developer_instructions (checked in the rollout); Pi receives them via --append-system-prompt (checked in every provider request). A role change rejects continuation of existing ExternalSubagents.",
			"write_behavior":   "every ExternalSubagent call is read-only unless it sets write; write scope (isolation worktree, or writable_roots) is chosen per call and fixed for that ExternalSubagent; Pi never writes",
			"connections":      subagents.Connections(home),
			"host_effective":   "enabled means the Claude user MCP entry agentx is registered. connections lists live AgentX MCP processes and whether each host completed initialization. Empty means no active server observed. Channel delivery requires interactive Claude with named opt-in and feature/org approval; initialization alone cannot verify that host-controlled gate. Confirm channel registration in Claude debug log and an actual completion report. Restart/reconnect after enable or preset changes to refresh the initialization preset list; ax claude supplies --dangerously-load-development-channels server:agentx; direct claude needs that flag.",
			"disable_behavior": "removes the Claude user MCP entry: new Claude sessions do not start AgentX; established connections run until their session ends and their work can finish; StopExternalSubagent remains available there", "session_scope": "current MCP connection only; reconnect loses actionable ExternalSubagent objects; no automatic replay",
		}, a.structuredDefault())
	default:
		return fmt.Errorf("unknown sub-agent-management action %q", args[0])
	}
}

func claudeConfigPath(home string) string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	return filepath.Join(home, ".claude.json")
}

func agentxEntry() (json.RawMessage, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"type": "stdio", "command": executable, "args": []string{"sub-agent-management", "serve"}})
}

// mcpRegistration reports whether Claude's user configuration registers this executable as agentx;
// that registration is the only enabled state. A different agentx entry is an error, never replaced.
func mcpRegistration(home string) (bool, error) {
	entry, err := agentxEntry()
	if err != nil {
		return false, err
	}
	var root struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	data, err := os.ReadFile(claudeConfigPath(home))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("read Claude user configuration %s: %w", claudeConfigPath(home), err)
	}
	existing, ok := root.MCPServers["agentx"]
	if !ok {
		return false, nil
	}
	var actual, want any
	if err := json.Unmarshal(existing, &actual); err != nil {
		return false, err
	}
	json.Unmarshal(entry, &want)
	if !reflect.DeepEqual(actual, want) {
		return false, fmt.Errorf("Claude user MCP entry agentx in %s has a different configuration; nothing changed. Resolve the conflict explicitly", claudeConfigPath(home))
	}
	return true, nil
}

func (a App) presetHome() string {
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		return override
	}
	return a.Hooks.Home
}

func (a App) prepareMCPSubagents(request *runtime.RunRequest, explicit bool) error {
	registered, err := mcpRegistration(a.presetHome())
	if err != nil {
		return err
	}
	if !registered {
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
