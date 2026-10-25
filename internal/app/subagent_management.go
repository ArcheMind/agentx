package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/ArcheMind/agentx/internal/profiles"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/subagents"
)

func (a App) profile(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return profileUsageError()
	}
	home := a.profileHome()
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: ax [--json|--yaml] profile list")
		}
		return a.profileList(home)
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: ax [--json|--yaml] profile show <name>")
		}
		return a.profileShow(home, args[1])
	default:
		return profileUsageError()
	}
}

func (a App) mcpServe(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return (&subagents.Server{Home: a.profileHome(), Cwd: cwd, Runner: a.Runner}).Serve(ctx, a.Stdin, a.Stdout)
}

func profileUsageError() error {
	return fmt.Errorf("usage: ax [--json|--yaml] profile <list|show> [args...]")
}

func profileEntry(o profiles.Profile) map[string]any {
	reason := "static requirements satisfied; login and model access verified per execution"
	if err := subagents.Ready(o); err != nil {
		reason = err.Error()
	}
	entry := map[string]any{"name": o.Name, "agent": o.Agent, "model": o.Model, "effort": profiles.EffortLabel(o.Effort), "description": o.Description, "status": reason, "limits": subagents.Limits(o)}
	return entry
}

func (a App) profileList(home string) error {
	list, problems, err := profiles.Load(home)
	if err != nil {
		return err
	}
	options := []map[string]any{}
	for _, o := range list {
		options = append(options, profileEntry(o))
	}
	invalid := append([]profiles.Problem{}, problems...)
	if a.Output != OutputText {
		return writeStructured(a.Stdout, map[string]any{
			"profiles_directory": profiles.Dir(home),
			"profiles":           options,
			"invalid_profiles":   invalid,
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

func (a App) profileShow(home string, name string) error {
	list, problems, err := profiles.Load(home)
	if err != nil {
		return err
	}
	var found *profiles.Profile
	for i := range list {
		if list[i].Name == name {
			found = &list[i]
			break
		}
	}
	if found == nil {
		for _, p := range problems {
			if p.Name == name {
				return fmt.Errorf("profile %s is invalid: %s", name, p.Err)
			}
		}
		return fmt.Errorf("profile %q not found in %s", name, profiles.Dir(home))
	}
	result := profileEntry(*found)
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
	if err := json.Unmarshal(existing, &entry); err != nil || (entry.Type != "" && entry.Type != "stdio") || entry.Command != executable || !reflect.DeepEqual(entry.Args, []string{"mcp", "serve"}) || len(entry.Env) != 0 {
		return false, fmt.Errorf("Claude user MCP entry agentx in %s does not run %s mcp serve; AgentX does not use or change it. Resolve the conflict explicitly", claudeConfigPath(home), executable)
	}
	return true, nil
}

func (a App) profileHome() string {
	if override := os.Getenv("AX_SUBAGENT_HOME"); override != "" {
		return override
	}
	return a.Hooks.Home
}

func (a App) injectClaudeAgentX(request *runtime.RunRequest, explicit bool) error {
	registered, err := mcpRegistration(a.profileHome())
	if err != nil {
		return err
	}
	if !registered {
		if explicit {
			return fmt.Errorf("AgentX is not registered with Claude; run: claude mcp add --scope user agentx -- %s mcp serve", executableOrAx())
		}
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	entry, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"agentx": map[string]any{"type": "stdio", "command": executable, "args": []string{"mcp", "serve"}}}})
	request.PassthroughArgs = append([]string{
		"--mcp-config", string(entry),
		"--allowedTools", "mcp__agentx__ExternalSubagent", "mcp__agentx__SendExternalSubagentMessage", "mcp__agentx__StopExternalSubagent", "mcp__agentx__listExternalSubagentSessions",
	}, request.PassthroughArgs...)
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

func executableOrAx() string {
	if executable, err := os.Executable(); err == nil {
		return executable
	}
	return "ax"
}
