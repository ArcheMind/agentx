package agentparams

import (
	"path/filepath"
	"strings"
)

func buildSimple(agent string, t Transport, r Request) (Plan, error) {
	if t != Interactive {
		return unsupported(agent, t, "transport")
	}
	if r.Effort != "" {
		return unsupported(agent, t, "effort")
	}
	if r.Mode == ModeYolo {
		return unsupported(agent, t, "yolo mode")
	}
	if r.Session != "" && agent != "opencode" {
		return unsupported(agent, t, "session")
	}
	if r.SystemPrompt != "" {
		return unsupported(agent, t, "system prompt")
	}
	if r.UserConfig != nil && !*r.UserConfig {
		return unsupported(agent, t, "user config")
	}
	if len(r.MCP.Servers) > 0 {
		return unsupported(agent, t, "MCP servers")
	}
	if agent == "dsh" && r.Model != "" {
		return unsupported(agent, t, "model selection")
	}
	args := []string{}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	if r.Session != "" {
		if filepath.IsAbs(r.Session) || strings.ContainsAny(r.Session, `/\`) {
			return unsupported(agent, t, "imported session directories; a native session ID is required")
		}
		args = append(args, "--session", r.Session)
	}
	args = append(args, r.Flags...)
	return Plan{Args: args}, nil
}
