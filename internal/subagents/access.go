package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func resolveWritableRoots(paths []string) ([]string, error) {
	return paths, nil
}

// Auth is the native credential source discovered for one turn.
type Auth struct {
	Key, Dir, Method string
}

func (a Auth) String() string {
	return fmt.Sprintf("%s=%q (%s)", a.Key, a.Dir, a.Method)
}

// authSource is the native agent's own credential directory: its environment variable when set,
// otherwise the native default under the user's home. AX_SUBAGENT_HOME never changes it.
func authSource(agent string) (string, string, error) {
	key, rel := "CODEX_HOME", ".codex"
	if agent == "pi" {
		key, rel = "PI_CODING_AGENT_DIR", filepath.Join(".pi", "agent")
	}
	if dir := os.Getenv(key); dir != "" {
		if !filepath.IsAbs(dir) {
			return key, "", fmt.Errorf("%s=%q must be an absolute path; no fallback identity", key, dir)
		}
		return key, filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return key, "", fmt.Errorf("%s unset and home directory unknown: %w; no fallback identity", key, err)
	}
	return key, filepath.Join(home, rel), nil
}

func authEnvironment(auth Auth) (map[string]string, []string) {
	return map[string]string{auth.Key: auth.Dir}, nil
}

// discoverAuth runs the native login check against the discovered source before each execution
// so a missing or broken login fails loudly instead of the runtime falling back to another identity.
func (s *Server) discoverAuth(ctx context.Context, o presets.Preset) (Auth, error) {
	key, dir, err := authSource(o.Agent)
	if err != nil {
		return Auth{}, err
	}
	auth := Auth{Key: key, Dir: dir}
	env, unset := authEnvironment(auth)
	plan := runtime.CommandPlan{Executable: o.Agent, Env: env, UnsetEnv: unset, Args: []string{"login", "status"}}
	if o.Agent == "pi" {
		plan.Args = []string{"auth", "check", "--json", "--model", o.Model}
	}
	result, err := s.Runner.Execute(ctx, plan, runtime.ExecuteOptions{})
	if ctx.Err() != nil {
		return Auth{}, &executionCancelled{cause: ctx.Err()}
	}
	output := runtime.RedactSecrets(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
	if o.Agent == "pi" {
		var status struct {
			Status   string `json:"status"`
			Provider string `json:"provider"`
			AuthType string `json:"authType"`
			Reason   string `json:"reason"`
		}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &status); jsonErr != nil {
			return Auth{}, fmt.Errorf("Pi auth check for %s=%q failed: %v; output: %s; no fallback identity", key, dir, err, output)
		}
		provider, _, _ := strings.Cut(o.Model, "/")
		if status.Status != "ready" {
			return Auth{}, fmt.Errorf("Pi auth source %s=%q not ready for provider %s: %s; no fallback identity", key, dir, provider, status.Reason)
		}
		if status.Provider != provider || status.AuthType == "" {
			return Auth{}, fmt.Errorf("Pi auth source %s=%q reports provider %q with auth type %q for model provider %s; no fallback identity", key, dir, status.Provider, status.AuthType, provider)
		}
		auth.Method = status.AuthType
		return auth, nil
	}
	switch {
	case strings.Contains(output, "Logged in using ChatGPT"):
		auth.Method = "chatgpt"
	case strings.Contains(output, "Logged in using an API key"):
		auth.Method = "api-key"
	}
	if err != nil || auth.Method == "" {
		return Auth{}, fmt.Errorf("Codex auth source %s=%q is not usable: %s; no fallback identity", key, dir, output)
	}
	return auth, nil
}

func Limits(o presets.Preset) string {
	if !presets.Agents[o.Agent].Subagent {
		return "Not callable as ExternalSubagent; usable for interactive ax launch only."
	}
	var text strings.Builder
	if o.Agent == "pi" {
		text.WriteString("Writes: not supported (agent has no runtime write boundary). Pi runs read-only through its tool allowlist (read, grep, find, ls; no bash, edit or write); this is an application-level tool filter, not an OS sandbox; reads are not path-restricted; a write request is rejected because Pi has no runtime write boundary. User and project extensions, skills, prompt templates and project approval disabled; with a role or explicit effort AgentX loads only its own observer extension, which records each provider request's effort and role inclusion in the Pi session and changes nothing. No approval path; Claude permissions and approvals are not inherited. ")
		text.WriteString("Auth: PI_CODING_AGENT_DIR (or ~/.pi/agent), checked by pi auth check for the model's provider before each execution; the report names the directory and auth type. ")
	} else {
		text.WriteString("Writes: read-only unless the call requests write, enforced by Codex native OS sandbox. Without write: no file writes. With write and isolation \"worktree\": the worktree is the only writable root. With write and writable_roots: In write mode the MCP working directory is always writable as well (native Codex behavior); writable_roots grants additional directories. .git inside writable roots stays read-only; /tmp, $TMPDIR and network access from commands are blocked; apply_patch outside the roots is rejected by Codex application-level patch approval; reads are not path-restricted. Sandbox denials are returned to the agent; there is no human approval path; Claude permissions and approvals are not inherited. Codex built-in web search remains available. ")
		text.WriteString("Auth: CODEX_HOME (or ~/.codex), checked by codex login status before each execution; the report names the directory and login method. ")
	}
	text.WriteString("Auth identity means that directory plus its login method; the account inside it is not verified. No fallback identity. ")
	text.WriteString(roleLimits(o.Agent))
	text.WriteString(IsolationLimits(o.Agent))
	if o.Agent == "pi" {
		text.WriteString("Effort is fixed by the preset (--thinking low, medium, high, xhigh or max; off and minimal rejected). Each turn fails unless the Pi session's thinking level and every provider request's reasoning effort equal it, so a silent clamp or remap is a failure, not a downgrade; models whose requests carry no reasoning effort field fail explicit effort. Exact provider/model required. ")
	} else {
		text.WriteString("Effort is fixed by the preset; each turn fails unless every completed model response in Codex telemetry reports exactly that effort (for example gpt-5.6-luna silently runs ultra as max); native rejection of an unsupported level is returned as failure. ")
	}
	text.WriteString("Model availability is verified only by execution.")
	return text.String()
}

// checkWrite rejects write requests whose scope would be ambiguous or unenforceable before any
// work starts; without write every run is read-only.
func checkWrite(o presets.Preset, in ExternalSubagentInput) error {
	switch {
	case !in.Write && in.WritableRoots != nil:
		return fmt.Errorf("writable_roots requires write: true; no task started")
	case !in.Write:
		return nil
	case !presets.Agents[o.Agent].WriteCapable:
		return fmt.Errorf("write rejected for %s preset %s: %s has no runtime write boundary; no task started", presets.AgentName(o.Agent), o.Name, presets.AgentName(o.Agent))
	case in.Isolation == IsolationWorktree && in.WritableRoots != nil:
		return fmt.Errorf("writable_roots is not accepted with isolation %q: the worktree is the only writable root; no task started", IsolationWorktree)
	case in.Isolation != IsolationWorktree && len(in.WritableRoots) == 0:
		return fmt.Errorf("write without isolation requires writable_roots (absolute directories); no task started")
	}
	return nil
}

func (a *ExternalSubagentState) writeScope() string {
	if a.Worktree != nil {
		if len(a.Worktree.Roots) == 0 {
			return "read-only (no write requested)"
		}
		return fmt.Sprintf("worktree only %q", a.Worktree.Roots)
	}
	if len(a.Roots) == 0 {
		return "read-only (no write requested)"
	}
	return fmt.Sprintf("working directory %q plus writable roots %q. In write mode the MCP working directory is always writable as well (native Codex behavior); writable_roots grants additional directories.", a.Cwd, a.Roots)
}
