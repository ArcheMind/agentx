package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/profiles"
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
	if agent == "claude" {
		return "", "", nil
	}
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
	if auth.Key == "" {
		return map[string]string{}, nil
	}
	return map[string]string{auth.Key: auth.Dir}, nil
}

// discoverAuth runs the native login check against the discovered source before each execution
// so a missing or broken login fails loudly instead of the runtime falling back to another identity.
func (s *Server) discoverAuth(ctx context.Context, o profiles.Profile) (Auth, error) {
	if o.Agent == "claude" {
		return Auth{Method: "native"}, nil
	}
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

func Limits(o profiles.Profile) string {
	var text strings.Builder
	text.WriteString("Write: read-only unless the call sets write; write scope is passed to the agent's native sandbox. ")
	text.WriteString("With isolation \"worktree\": writable_roots is not accepted. Without isolation: write requires writable_roots. ")
	text.WriteString("Profile sandbox, user_config and flags are passed through to the agent. ")
	text.WriteString("Auth is discovered before each execution; the report names the source. ")
	text.WriteString(IsolationLimits())
	text.WriteString("Effort is fixed by the profile. Model availability is verified only by execution.")
	return text.String()
}

// checkWrite rejects write requests whose scope would be ambiguous or unenforceable before any
// work starts; without write every run is read-only.
func checkWrite(o profiles.Profile, in ExternalSubagentInput) error {
	switch {
	case !in.Write && in.WritableRoots != nil:
		return fmt.Errorf("writable_roots requires write: true; no task started")
	case !in.Write:
		return nil
	case !profiles.Agents[o.Agent].WriteCapable:
		return fmt.Errorf("write rejected for %s profile %s: %s has no runtime write boundary; no task started", profiles.AgentName(o.Agent), o.Name, profiles.AgentName(o.Agent))
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
