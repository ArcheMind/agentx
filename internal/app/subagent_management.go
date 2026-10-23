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
	if err := json.Unmarshal(existing, &entry); err != nil || (entry.Type != "" && entry.Type != "stdio") || entry.Command != executable || !reflect.DeepEqual(entry.Args, []string{"preset", "serve"}) || len(entry.Env) != 0 {
		return false, fmt.Errorf("Claude user MCP entry agentx in %s does not run %s preset serve; AgentX does not use or change it. Resolve the conflict explicitly", claudeConfigPath(home), executable)
	}
	return true, nil
}

func (a App) presetHome() string {
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		return override
	}
	return a.Hooks.Home
}

func (a App) injectClaudeAgentX(request *runtime.RunRequest, explicit bool) error {
	registered, err := mcpRegistration(a.presetHome())
	if err != nil {
		return err
	}
	if !registered {
		if explicit {
			return fmt.Errorf("AgentX is not registered with Claude; run: claude mcp add --scope user agentx -- %s preset serve", executableOrAx())
		}
		return nil
	}
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

func injectCodexAgentX(request *runtime.RunRequest) {
	bin, err := os.Executable()
	if err != nil {
		return
	}
	request.PassthroughArgs = append(request.PassthroughArgs,
		"-c", fmt.Sprintf("mcp_servers.agentx.command=%q", bin),
		"-c", `mcp_servers.agentx.args=["preset","serve"]`,
	)
	if subHome := os.Getenv("AX_SUBAGENT_HOME"); subHome != "" {
		request.PassthroughArgs = append(request.PassthroughArgs,
			"-c", fmt.Sprintf("mcp_servers.agentx.env.AX_SUBAGENT_HOME=%q", subHome),
		)
	}
}

func executableOrAx() string {
	if executable, err := os.Executable(); err == nil {
		return executable
	}
	return "ax"
}
