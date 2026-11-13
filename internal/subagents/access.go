package subagents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/presets"
)

const (
	ModeNormal = agentparams.ModeNormal
	ModeYolo   = agentparams.ModeYolo
)

// Auth is the native credential source discovered for one turn.
type Auth struct {
	Key, Dir string
}

func (a Auth) String() string {
	return fmt.Sprintf("%s=%q", a.Key, a.Dir)
}

// authSource is the native agent's own credential directory: its environment variable when set,
// otherwise the native default under the user's home. SM_SUBAGENT_HOME never changes it.
func authSource(agent string) (string, string, error) {
	if agent == "claude" || agent == "agy" {
		return "", "", nil
	}
	if agent != "codex" && agent != "pi" {
		return "", "", fmt.Errorf("%s has no native credential source", agent)
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

// DiscoverAuth pins the execution to the native agent's own credential directory.
// Whether that directory can serve the model is the runtime's own call: pi and
// codex resolve models and credentials themselves and fail loudly in-stream.
func DiscoverAuth(agent string) (Auth, error) {
	if agent == "claude" || agent == "agy" {
		return Auth{}, nil
	}
	key, dir, err := authSource(agent)
	if err != nil {
		return Auth{}, err
	}
	return Auth{Key: key, Dir: dir}, nil
}

func Limits(o presets.Preset) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Mode: preset default is %s. ", presets.ModeLabel(o.Mode))
	text.WriteString("normal passes no permission or sandbox flags, so the agent's own configuration decides; yolo bypasses the agent's permission checks and sandbox. ")
	if !agentparams.Agents[o.Agent].Bypassable {
		text.WriteString("This agent rejects yolo. ")
	}
	text.WriteString("Preset user_config and flags are passed through to the agent. ")
	text.WriteString("The native credential directory is pinned before each execution; the report names it. ")
	text.WriteString(IsolationLimits())
	text.WriteString("Effort is fixed by the preset.")
	return text.String()
}
