package agentparams

func buildPi(t Transport, r Request) (Plan, error) {
	if t != Interactive && t != OneShot && t != RPC && t != Exec {
		return unsupported("pi", t, "transport")
	}
	if len(r.MCP.Servers) > 0 {
		return unsupported("pi", t, "MCP servers")
	}
	if t == Exec {
		if err := requireUUID("Pi exec resume", r.Session); err != nil {
			return Plan{}, err
		}
	}
	args := []string{}
	if t == OneShot {
		args = append(args, "--print", "--mode", "json", "--no-approve")
	}
	if t == RPC {
		args = append(args, "--mode", "rpc", "--no-approve")
	}
	if t == Exec {
		args = append(args, "--print", "--mode", "json")
	}
	if r.Session != "" {
		args = append(args, "--session", r.Session)
	}
	if r.Effort != "" {
		args = append(args, "--thinking", r.Effort)
	}
	if r.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", r.SystemPrompt)
	}
	if r.UserConfig != nil && !*r.UserConfig {
		args = append(args, "--no-skills", "--no-prompt-templates")
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	args = append(args, r.Flags...)
	return Plan{Args: args}, nil
}
