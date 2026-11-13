package agentparams

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	ModeNormal  = "normal"
	ModeYolo    = "yolo"
	ModeDefault = ModeYolo
)

type AgentRule struct {
	Name       string
	Efforts    []string
	EffortNote string
	Bypassable bool
}

var Agents = map[string]AgentRule{
	"agy":      {Name: "Antigravity CLI", Efforts: []string{"low", "medium", "high", "xhigh", "max"}, EffortNote: "Antigravity CLI --effort levels; per-model support is not verified by agy models", Bypassable: true},
	"claude":   {Name: "Claude", Efforts: []string{"low", "medium", "high", "xhigh", "max"}, EffortNote: "Claude Code --effort levels; per-model support is not verified locally", Bypassable: true},
	"codex":    {Name: "Codex", Efforts: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}, EffortNote: "Codex reports supported levels per model", Bypassable: true},
	"opencode": {Name: "OpenCode", EffortNote: "OpenCode exposes no verified effort control"},
	"pi":       {Name: "Pi", Efforts: []string{"low", "medium", "high", "xhigh", "max"}, EffortNote: "Pi --thinking levels; ExternalSubagent reports a warning when Pi's provider request carries a different effort, without blocking execution (Pi can clamp or remap unsupported levels). off and minimal are not offered: for Codex-backed models Pi sends no effort for off (the server default applies) and sends low for minimal", Bypassable: true},
}
var AgentOrder = []string{"claude", "codex", "opencode", "pi", "agy"}

type Request struct {
	Cwd, Model, Effort, Mode, Session, SystemPrompt string
	UserConfig                                      *bool
	Flags                                           []string
	MCP                                             MCPConfig
}
type Transport string

const (
	Interactive Transport = "interactive"
	OneShot     Transport = "one-shot"
	Stream      Transport = "stream"
	AppServer   Transport = "app-server"
	RPC         Transport = "rpc"
	ACP         Transport = "acp"
	Exec        Transport = "exec"
)

type Setting struct {
	Method string
	Params map[string]any
}
type Plan struct {
	Args         []string
	Params       map[string]any
	Settings     []Setting
	PromptPrefix string
}

var sessionID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func EffectiveMode(agent, mode string) string {
	if mode != "" {
		return mode
	}
	if rule, ok := Agents[agent]; ok && !rule.Bypassable {
		return ModeNormal
	}
	return ModeDefault
}

func Resolve(agent string, base, override Request) (Request, error) {
	r := base
	if override.Cwd != "" {
		r.Cwd = override.Cwd
	}
	if override.Model != "" {
		r.Model = override.Model
	}
	if override.Effort != "" {
		r.Effort = override.Effort
	}
	if override.Mode != "" {
		r.Mode = override.Mode
	}
	if override.Session != "" {
		r.Session = override.Session
	}
	if override.SystemPrompt != "" {
		r.SystemPrompt = override.SystemPrompt
	}
	if override.UserConfig != nil {
		r.UserConfig = override.UserConfig
	}
	if override.Flags != nil {
		r.Flags = append([]string{}, override.Flags...)
	}
	if override.MCP.Servers != nil {
		r.MCP = override.MCP
	}
	return r, Validate(agent, r)
}

func Validate(agent string, r Request) error {
	rule, ok := Agents[agent]
	if !ok && agent != "dsh" {
		return fmt.Errorf("unsupported agent %q", agent)
	}
	if strings.ContainsFunc(r.Model, func(c rune) bool { return c <= ' ' || c == 0x7f }) {
		return fmt.Errorf("model %q must not contain whitespace or control characters", r.Model)
	}
	if r.Mode != "" && r.Mode != ModeNormal && r.Mode != ModeYolo {
		return fmt.Errorf("unsupported mode %q", r.Mode)
	}
	if r.Mode == ModeYolo && !rule.Bypassable {
		return fmt.Errorf("mode %q is not supported for %s", r.Mode, agent)
	}
	if r.Effort != "" && !slices.Contains(rule.Efforts, r.Effort) {
		return fmt.Errorf("effort %q is not supported for %s", r.Effort, agent)
	}
	for name, server := range r.MCP.Servers {
		if name == "" || server.Command == "" {
			return fmt.Errorf("MCP server %q requires a name and command", name)
		}
	}
	return nil
}

func Build(agent string, transport Transport, r Request) (Plan, error) {
	if err := Validate(agent, r); err != nil {
		return Plan{}, err
	}
	if err := checkConflicts(agent, r); err != nil {
		return Plan{}, err
	}
	switch agent {
	case "claude":
		return buildClaude(transport, r)
	case "codex":
		return buildCodex(transport, r)
	case "agy":
		return buildAgy(transport, r)
	case "pi":
		return buildPi(transport, r)
	case "opencode", "dsh":
		return buildSimple(agent, transport, r)
	}
	return Plan{}, fmt.Errorf("unsupported agent %q", agent)
}

func unsupported(agent string, t Transport, field string) (Plan, error) {
	return Plan{}, fmt.Errorf("%s %s does not support %s", agent, t, field)
}
func requireUUID(agent, session string) error {
	if session != "" && !sessionID.MatchString(session) {
		return fmt.Errorf("%s requires an exact session UUID", agent)
	}
	return nil
}

func checkConflicts(agent string, r Request) error {
	active := map[string]bool{"model": r.Model != "", "effort": r.Effort != "", "mode": r.Mode != "", "session": r.Session != "", "system prompt": r.SystemPrompt != "", "user config": r.UserConfig != nil, "MCP": len(r.MCP.Servers) > 0}
	aliases := map[string]string{"--model": "model"}
	modeFlags := map[string]bool{}
	switch agent {
	case "claude":
		aliases["-m"] = "model"
		aliases["--effort"] = "effort"
		aliases["--resume"] = "session"
		aliases["-r"] = "session"
		aliases["--append-system-prompt"] = "system prompt"
		aliases["--system-prompt"] = "system prompt"
		aliases["--setting-sources"] = "user config"
		aliases["--mcp-config"] = "MCP"
		aliases["--strict-mcp-config"] = "MCP"
		modeFlags["--dangerously-skip-permissions"] = true
		modeFlags["--approval-mode"] = true
		modeFlags["--permission-mode"] = true
	case "codex":
		aliases["-m"] = "model"
		aliases["--ignore-user-config"] = "user config"
		for _, flag := range []string{"--dangerously-bypass-approvals-and-sandbox", "--ask-for-approval", "-a", "--sandbox", "-s", "--approve-for-me", "--full-auto"} {
			modeFlags[flag] = true
		}
	case "agy":
		aliases["--effort"] = "effort"
		aliases["--conversation"] = "session"
		aliases["--include-default-customizations"] = "user config"
		modeFlags["--dangerously-skip-permissions"] = true
	case "opencode":
		aliases["-m"] = "model"
		aliases["--session"] = "session"
		aliases["-s"] = "session"
		aliases["--continue"] = "session"
		aliases["-c"] = "session"
	case "pi":
		aliases["--thinking"] = "effort"
		aliases["--session"] = "session"
		aliases["--append-system-prompt"] = "system prompt"
		aliases["--no-skills"] = "user config"
		aliases["--no-prompt-templates"] = "user config"
	}
	valueFlags := map[string]bool{"--model": true, "-m": true, "--effort": true, "--thinking": true, "--resume": true, "-r": true, "--session": true, "--conversation": true, "--append-system-prompt": true, "--system-prompt": true, "--setting-sources": true, "--include-default-customizations": true, "--mcp-config": true, "--approval-mode": true, "--permission-mode": true, "--ask-for-approval": true, "-a": true, "--sandbox": true, "-s": true, "--prompt": true, "--tools": true, "--output-format": true, "--input-format": true, "--mode": true}
	for i := 0; i < len(r.Flags); i++ {
		flag := r.Flags[i]
		if flag == "--" {
			break
		}
		if !strings.HasPrefix(flag, "-") || flag == "-" {
			continue
		}
		name, inline, hasInline := strings.Cut(flag, "=")
		if agent == "codex" && (name == "-c" || name == "--config") {
			value := inline
			if !hasInline && i+1 < len(r.Flags) {
				i++
				value = r.Flags[i]
			}
			if field := configConflict(value, r); field != "" {
				return fmt.Errorf("%s config %q conflicts with explicit %s", agent, value, field)
			}
			continue
		}
		field := aliases[name]
		if modeFlags[name] {
			field = "mode"
		}
		if active[field] {
			return fmt.Errorf("%s flag %q conflicts with explicit %s", agent, flag, field)
		}
		if valueFlags[name] && !hasInline && i+1 < len(r.Flags) {
			i++
		}
	}
	return nil
}

func configConflict(raw string, r Request) string {
	key, _, ok := strings.Cut(raw, "=")
	if !ok {
		return ""
	}
	key = strings.TrimSpace(key)
	parts := strings.Split(key, ".")
	for i := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(parts[i]), `"'`)
	}
	key = strings.Join(parts, ".")
	switch key {
	case "model":
		if r.Model != "" {
			return "model"
		}
	case "model_reasoning_effort":
		if r.Effort != "" {
			return "effort"
		}
	case "approval_policy", "sandbox_mode":
		if r.Mode != "" {
			return "mode"
		}
	case "developer_instructions":
		if r.SystemPrompt != "" {
			return "system prompt"
		}
	}
	if strings.HasPrefix(key, "mcp_servers.") {
		name := strings.Split(strings.TrimPrefix(key, "mcp_servers."), ".")[0]
		if _, ok := r.MCP.Servers[name]; ok {
			return "MCP"
		}
	}
	return ""
}
