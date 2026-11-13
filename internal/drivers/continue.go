package drivers

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/ArcheMind/agentx/internal/runtime"
)

// MCPServer is one stdio MCP server to mount into a continued turn.
// Env carries explicit values; EnvPassthrough names variables the agent should
// forward from its own environment (Codex spawns MCP servers with a clean
// environment, so inheritance alone is not enough there).
type MCPServer struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	EnvPassthrough []string          `json:"envPassthrough,omitempty"`
}

// MCPConfig is the unified MCP mount request translated per agent.
type MCPConfig struct {
	Servers map[string]MCPServer `json:"mcpServers"`
}

// ContinueExecPlan builds the headless one-turn invocation for `ax session continue`.
// The prompt travels on stdin. resume is the token WriteSessionFile returned: a
// session UUID for claude/codex, a session file path for pi. Mode follows the
// two-mode permission surface: normal passes no permission or sandbox flags,
// yolo bypasses them.
func ContinueExecPlan(agent, cwd, model, resume, effort, systemPrompt string, yolo bool, mcp MCPConfig) (runtime.CommandPlan, error) {
	switch agent {
	case "claude":
		return continueClaude(cwd, model, resume, effort, systemPrompt, yolo, mcp)
	case "codex":
		return continueCodex(cwd, model, resume, effort, systemPrompt, yolo, mcp)
	case "agy":
		return continueAgy(cwd, model, resume, effort, systemPrompt, yolo, mcp)
	case "pi":
		return continuePi(cwd, model, resume, effort, systemPrompt, yolo, mcp)
	default:
		return runtime.CommandPlan{}, fmt.Errorf("%s has no headless continue adapter", agent)
	}
}

func continueClaude(cwd, model, resume, effort, systemPrompt string, yolo bool, mcp MCPConfig) (runtime.CommandPlan, error) {
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if resume != "" {
		if !sessionID.MatchString(resume) {
			return runtime.CommandPlan{}, fmt.Errorf("continue requires a Claude session UUID")
		}
		args = append(args, "--resume", resume)
	}
	if yolo {
		args = append(args, "--dangerously-skip-permissions")
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	if len(mcp.Servers) > 0 {
		servers := map[string]any{}
		for name, server := range mcp.Servers {
			entry := map[string]any{"type": "stdio", "command": server.Command}
			if len(server.Args) > 0 {
				entry["args"] = server.Args
			}
			// Claude passes its own full environment to stdio MCP servers, so
			// EnvPassthrough needs no translation here.
			if len(server.Env) > 0 {
				entry["env"] = server.Env
			}
			servers[name] = entry
		}
		config, err := json.Marshal(map[string]any{"mcpServers": servers})
		if err != nil {
			return runtime.CommandPlan{}, err
		}
		// strict keeps the mounted set exactly what the caller asked for,
		// independent of the user's own MCP configuration.
		args = append(args, "--mcp-config", string(config), "--strict-mcp-config")
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return runtime.CommandPlan{Executable: "claude", Args: args, Cwd: cwd}, nil
}

func continueCodex(cwd, model, resume, effort, systemPrompt string, yolo bool, mcp MCPConfig) (runtime.CommandPlan, error) {
	args := []string{"exec"}
	if resume != "" {
		if !sessionID.MatchString(resume) {
			return runtime.CommandPlan{}, fmt.Errorf("continue requires a Codex session UUID")
		}
		args = append(args, "resume")
	}
	args = append(args, "--json", "--skip-git-repo-check")
	if yolo {
		args = append(args, "-c", `sandbox_mode="danger-full-access"`)
	}
	// approval_policy="never" always: a headless run cannot answer prompts.
	args = append(args, "-c", `approval_policy="never"`)
	if effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
	}
	if systemPrompt != "" {
		args = append(args, "-c", fmt.Sprintf("developer_instructions=%q", systemPrompt))
	}
	for _, name := range sortedServerNames(mcp) {
		server := mcp.Servers[name]
		args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.command=%q", name, server.Command))
		if len(server.Args) > 0 {
			encoded, err := json.Marshal(server.Args)
			if err != nil {
				return runtime.CommandPlan{}, err
			}
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.args=%s", name, encoded))
		}
		for _, key := range sortedKeys(server.Env) {
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.env.%s=%q", name, key, server.Env[key]))
		}
		if len(server.EnvPassthrough) > 0 {
			encoded, err := json.Marshal(server.EnvPassthrough)
			if err != nil {
				return runtime.CommandPlan{}, err
			}
			args = append(args, "-c", fmt.Sprintf("mcp_servers.%s.env_vars=%s", name, encoded))
		}
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if resume != "" {
		args = append(args, resume)
	}
	args = append(args, "-")
	return runtime.CommandPlan{Executable: "codex", Args: args, Cwd: cwd}, nil
}

func continuePi(cwd, model, resume, effort, systemPrompt string, yolo bool, mcp MCPConfig) (runtime.CommandPlan, error) {
	if len(mcp.Servers) > 0 {
		return runtime.CommandPlan{}, fmt.Errorf("pi has no MCP server mechanism; --mcp-config is not supported for pi")
	}
	// --no-approve always: a headless pi would otherwise hang on tool approval.
	// yolo is a no-op: Pi has no sandbox beyond --no-approve which is always set.
	args := []string{"--print", "--mode", "json", "--no-approve"}
	if resume != "" {
		args = append(args, "--session", resume)
	}
	if effort != "" {
		args = append(args, "--thinking", effort)
	}
	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return runtime.CommandPlan{Executable: "pi", Args: args, Cwd: cwd}, nil
}

func sortedServerNames(mcp MCPConfig) []string {
	names := make([]string, 0, len(mcp.Servers))
	for name := range mcp.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// AgyMCPServer is the native ACP stdio server shape.
type AgyMCPServer struct {
	Name    string      `json:"name"`
	Command string      `json:"command"`
	Args    []string    `json:"args"`
	Env     []AgyMCPEnv `json:"env"`
}
type AgyMCPEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func AgyMCPServers(config MCPConfig) []AgyMCPServer {
	result := make([]AgyMCPServer, 0, len(config.Servers))
	for _, name := range sortedServerNames(config) {
		server := config.Servers[name]
		env := map[string]string{}
		for _, key := range server.EnvPassthrough {
			if value, ok := os.LookupEnv(key); ok {
				env[key] = value
			}
		}
		for key, value := range server.Env {
			env[key] = value
		}
		entry := AgyMCPServer{Name: name, Command: server.Command, Args: append([]string{}, server.Args...), Env: []AgyMCPEnv{}}
		for _, key := range sortedKeys(env) {
			entry.Env = append(entry.Env, AgyMCPEnv{Name: key, Value: env[key]})
		}
		result = append(result, entry)
	}
	return result
}

func continueAgy(cwd, model, resume, effort, systemPrompt string, yolo bool, mcp MCPConfig) (runtime.CommandPlan, error) {
	if len(mcp.Servers) > 0 {
		return runtime.CommandPlan{}, fmt.Errorf("agy ordinary CLI has no verified temporary MCP mount; use its ACP session mcpServers")
	}
	if systemPrompt != "" {
		return runtime.CommandPlan{}, fmt.Errorf("agy ordinary CLI has no verified append-system-prompt mechanism")
	}
	args := []string{"--print", "--output-format", "stream-json"}
	if resume != "" {
		if !sessionID.MatchString(resume) {
			return runtime.CommandPlan{}, fmt.Errorf("continue requires an agy conversation UUID")
		}
		args = append(args, "--conversation", resume)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	if yolo {
		args = append(args, "--dangerously-skip-permissions")
	}
	return runtime.CommandPlan{Executable: "agy", Args: args, Cwd: cwd}, nil
}
