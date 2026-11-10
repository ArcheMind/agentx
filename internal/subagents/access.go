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

const (
	ModeNormal = profiles.ModeNormal
	ModeYolo   = profiles.ModeYolo
)

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

// AuthEnvironment is the environment override that pins an execution to the
// discovered credential source.
func AuthEnvironment(auth Auth) (map[string]string, []string) {
	if auth.Key == "" {
		return map[string]string{}, nil
	}
	return map[string]string{auth.Key: auth.Dir}, nil
}

func (s *Server) discoverAuth(ctx context.Context, o profiles.Profile) (Auth, error) {
	return DiscoverAuth(ctx, s.Runner, o.Agent, o.Model)
}

// DiscoverAuth runs the native login check against the discovered source before each execution
// so a missing or broken login fails loudly instead of the runtime falling back to another identity.
func DiscoverAuth(ctx context.Context, runner runtime.Runner, agent, model string) (Auth, error) {
	if agent == "claude" {
		return Auth{Method: "native"}, nil
	}
	key, dir, err := authSource(agent)
	if err != nil {
		return Auth{}, err
	}
	auth := Auth{Key: key, Dir: dir}
	env, unset := AuthEnvironment(auth)
	plan := runtime.CommandPlan{Executable: agent, Env: env, UnsetEnv: unset, Args: []string{"login", "status"}}
	if agent == "pi" {
		plan.Args = []string{"auth", "check", "--json", "--model", model}
	}
	result, err := runner.Execute(ctx, plan, runtime.ExecuteOptions{})
	if ctx.Err() != nil {
		return Auth{}, &executionCancelled{cause: ctx.Err()}
	}
	output := runtime.RedactSecrets(strings.TrimSpace(result.Stdout + "\n" + result.Stderr))
	if agent == "pi" {
		var status struct {
			Status   string `json:"status"`
			Provider string `json:"provider"`
			AuthType string `json:"authType"`
			Reason   string `json:"reason"`
		}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &status); jsonErr != nil {
			return Auth{}, fmt.Errorf("Pi auth check for %s=%q failed: %v; output: %s; no fallback identity", key, dir, err, output)
		}
		provider, _, _ := strings.Cut(model, "/")
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
	fmt.Fprintf(&text, "Mode: profile default is %s. ", profiles.ModeLabel(o.Mode))
	text.WriteString("normal passes no permission or sandbox flags, so the agent's own configuration decides; yolo bypasses the agent's permission checks and sandbox. ")
	if !profiles.Agents[o.Agent].Bypassable {
		text.WriteString("This agent rejects yolo. ")
	}
	text.WriteString("Profile user_config and flags are passed through to the agent. ")
	text.WriteString("Auth is discovered before each execution; the report names the source. ")
	text.WriteString(IsolationLimits())
	text.WriteString("Effort is fixed by the profile. Model availability is verified only by execution.")
	return text.String()
}

// effectiveMode resolves the call-time mode against the profile default.
func effectiveMode(o profiles.Profile, callMode string) string {
	if callMode != "" {
		return callMode
	}
	return o.EffectiveMode()
}

// checkMode rejects yolo before any work starts for agents without a permission boundary to bypass.
func checkMode(o profiles.Profile, mode string) error {
	if mode == ModeYolo && !profiles.Agents[o.Agent].Bypassable {
		return fmt.Errorf("mode %q rejected for %s profile %s: %s has no runtime permission boundary to bypass; no task started", ModeYolo, profiles.AgentName(o.Agent), o.Name, profiles.AgentName(o.Agent))
	}
	return nil
}
