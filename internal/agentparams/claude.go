package agentparams

func buildClaude(t Transport, r Request) (Plan, error) {
	if t != Interactive && t != OneShot && t != Stream {
		return unsupported("claude", t, "transport")
	}
	if err := requireUUID("Claude resume", r.Session); err != nil {
		return Plan{}, err
	}
	args := []string{}
	if t == OneShot {
		args = append(args, "-p", "--output-format", "stream-json", "--verbose")
	}
	if t == Stream {
		args = append(args, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose")
	}
	if r.Session != "" {
		args = append(args, "--resume", r.Session)
	}
	if r.Mode == ModeYolo {
		args = append(args, "--dangerously-skip-permissions")
	}
	if r.UserConfig != nil && !*r.UserConfig {
		args = append(args, "--setting-sources", "project,local")
	}
	if r.Effort != "" {
		args = append(args, "--effort", r.Effort)
	}
	if r.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", r.SystemPrompt)
	}
	if len(r.MCP.Servers) > 0 {
		config, err := claudeMCP(r.MCP)
		if err != nil {
			return Plan{}, err
		}
		args = append(args, "--mcp-config", config)
		if t == OneShot {
			args = append(args, "--strict-mcp-config")
		}
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	args = append(args, r.Flags...)
	return Plan{Args: args}, nil
}
