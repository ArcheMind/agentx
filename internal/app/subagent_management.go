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
		return fmt.Errorf("usage: ax sub-agent-management <status|serve> (presets are managed from bare ax; register with Claude using claude mcp add --scope user agentx -- <absolute path of ax> sub-agent-management serve)")
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
	case "status":
		registered, registrationErr := mcpRegistration(home)
		problem := ""
		if registrationErr != nil {
			problem = registrationErr.Error()
		}
		list, problems, err := presets.Load(home)
		if err != nil {
			return err
		}
		options := []map[string]any{}
		for _, o := range list {
			if !presets.Agents[o.Agent].Subagent {
				continue
			}
			reason := "static requirements satisfied; login and model access verified per execution"
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
			"mcp_registered": registered, "mcp_registration_problem": problem, "mcp_entry": claudeConfigPath(home), "mcp_register_command": "claude mcp add --scope user agentx -- <absolute path of ax> sub-agent-management serve", "presets_directory": presets.Dir(home), "presets": options, "invalid_presets": append([]presets.Problem{}, problems...),
			"roles_directory": roles.Dir(home), "roles": declared, "invalid_roles": append([]presets.Problem{}, roleProblems...),
			"role_behavior":       "subagent_type selects a role; its instructions are task framing only and never change agent, model, effort, write scope or permissions. Codex receives them as developer_instructions (checked in the rollout); Pi receives them via --append-system-prompt (checked in every provider request). A role change rejects continuation of existing ExternalSubagents.",
			"write_behavior":      "every ExternalSubagent call is read-only unless it sets write; write scope (isolation worktree, or writable_roots) is chosen per call and fixed for that ExternalSubagent; Pi never writes",
			"connections":         subagents.Connections(home),
			"host_effective":      "mcp_registered means Claude's user MCP entry agentx runs this ax with sub-agent-management serve. connections lists live AgentX MCP processes and whether each host completed initialization. Empty means no active server observed. Channel delivery requires interactive Claude with named opt-in and feature/org approval; initialization alone cannot verify that host-controlled gate. Confirm channel registration in Claude debug log and an actual completion report. Restart/reconnect after registering or after preset changes to refresh the initialization preset list; ax claude supplies --dangerously-load-development-channels server:agentx; direct claude needs that flag.",
			"unregister_behavior": "claude mcp remove --scope user agentx: new Claude sessions do not start AgentX; established connections run until their session ends and their work can finish; StopExternalSubagent remains available there", "session_scope": "session-scoped like native subagents: ExternalSubagents belong to the MCP connection that created them; a reconnect starts with none and old references fail explicitly",
		}, a.structuredDefault())
	default:
		return fmt.Errorf("unknown sub-agent-management action %q; actions: status, serve. Claude owns the MCP registration: claude mcp add --scope user agentx -- %s sub-agent-management serve (remove with claude mcp remove --scope user agentx)", args[0], executableOrAx())
	}
}

func claudeConfigPath(home string) string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json")
	}
	return filepath.Join(home, ".claude.json")
}

// mcpRegistration reports whether Claude's user configuration has an agentx entry that runs this
// executable as the AgentX server (claude mcp add also writes an empty env); any other agentx entry
// is a conflict and is never replaced.
func mcpRegistration(home string) (bool, error) {
	executable, err := os.Executable()
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
	var entry struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
	}
	if err := json.Unmarshal(existing, &entry); err != nil || (entry.Type != "" && entry.Type != "stdio") || entry.Command != executable || !reflect.DeepEqual(entry.Args, []string{"sub-agent-management", "serve"}) || len(entry.Env) != 0 {
		return false, fmt.Errorf("Claude user MCP entry agentx in %s does not run %s sub-agent-management serve; AgentX does not use or change it. Resolve the conflict explicitly", claudeConfigPath(home), executable)
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
			return fmt.Errorf("AgentX is not registered with Claude; run: claude mcp add --scope user agentx -- %s sub-agent-management serve", executableOrAx())
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

func executableOrAx() string {
	if executable, err := os.Executable(); err == nil {
		return executable
	}
	return "ax"
}
