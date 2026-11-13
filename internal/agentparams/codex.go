package agentparams

import (
	"fmt"
	"strings"
)

func buildCodex(t Transport, r Request) (Plan, error) {
	if t != Interactive && t != OneShot && t != AppServer {
		return unsupported("codex", t, "transport")
	}
	if err := requireUUID("Codex resume", r.Session); err != nil {
		return Plan{}, err
	}
	if r.UserConfig != nil && !*r.UserConfig && t != OneShot {
		return unsupported("codex", t, "user_config=false")
	}
	if t == AppServer {
		config := map[string]any{}
		if r.Effort != "" {
			config["model_reasoning_effort"] = r.Effort
		}
		if len(r.MCP.Servers) > 0 {
			config["mcp_servers"] = codexMCPConfig(r.MCP)
		}
		params := map[string]any{}
		if r.Cwd != "" {
			params["cwd"] = r.Cwd
		}
		if r.Model != "" {
			params["model"] = r.Model
		}
		if r.SystemPrompt != "" {
			params["developerInstructions"] = r.SystemPrompt
		}
		if len(config) > 0 {
			params["config"] = config
		}
		if r.Mode == ModeYolo {
			params["approvalPolicy"] = "never"
			params["sandbox"] = "danger-full-access"
		}
		if r.Session != "" {
			params["threadId"] = r.Session
			params["excludeTurns"] = true
		}
		return Plan{Args: append([]string{"app-server"}, r.Flags...), Params: params}, nil
	}
	args := []string{}
	if t == OneShot {
		args = append(args, "exec")
		if r.Session != "" {
			args = append(args, "resume")
		}
		args = append(args, "--json", "--skip-git-repo-check")
	}
	if t == Interactive && r.Session != "" {
		args = append(args, "resume", r.Session)
	}
	if r.Mode == ModeYolo {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	if t == OneShot && r.Mode != ModeYolo {
		args = append(args, "-c", `approval_policy="never"`)
	}
	if r.UserConfig != nil && !*r.UserConfig {
		args = append(args, "--ignore-user-config")
	}
	if r.Effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", r.Effort))
	}
	if r.SystemPrompt != "" {
		args = append(args, "-c", fmt.Sprintf("developer_instructions=%q", r.SystemPrompt))
	}
	mcp, err := codexMCPArgs(r.MCP)
	if err != nil {
		return Plan{}, err
	}
	args = append(args, mcp...)
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	args = append(args, r.Flags...)
	if t == OneShot {
		if r.Session != "" {
			args = append(args, r.Session)
		}
		args = append(args, "-")
	}
	return Plan{Args: args}, nil
}

// Config discovery must use the same native configuration selection as execution.
func CodexConfigArgs(r Request) ([]string, error) {
	if r.UserConfig != nil && !*r.UserConfig {
		return nil, fmt.Errorf("Codex app-server config discovery cannot honor user_config=false")
	}
	var args []string
	for i := 0; i < len(r.Flags); i++ {
		flag := r.Flags[i]
		if flag == "--" {
			break
		}
		switch {
		case flag == "-p" || flag == "--profile" || strings.HasPrefix(flag, "--profile=") || strings.HasPrefix(flag, "-p="):
			return nil, fmt.Errorf("Codex app-server config discovery cannot honor --profile")
		case flag == "--ignore-user-config":
			return nil, fmt.Errorf("Codex app-server config discovery cannot honor --ignore-user-config")
		case flag == "-c" || flag == "--config":
			if i+1 >= len(r.Flags) {
				return nil, fmt.Errorf("%s requires a value", flag)
			}
			args = append(args, flag, r.Flags[i+1])
			i++
		case strings.HasPrefix(flag, "--config="):
			args = append(args, flag)
		}
	}
	return args, nil
}
