package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ArcheMind/agentx/internal/runtime"
)

func authEnvironment(o Option, g Grant) (map[string]string, []string) {
	key := "CODEX_HOME"
	if o.Agent == "pi" {
		key = "PI_CODING_AGENT_DIR"
	}
	return map[string]string{key: g.Auth.Path}, runtime.CredentialEnvNames(os.Environ())
}

// verifyAuth checks the configured source before each execution so a missing, invalid or
// different login fails explicitly instead of the runtime falling back to another identity.
func (s *Server) verifyAuth(ctx context.Context, o Option, g Grant) error {
	env, unset := authEnvironment(o, g)
	plan := runtime.CommandPlan{Executable: o.Agent, Env: env, UnsetEnv: unset, Args: []string{"login", "status"}}
	if o.Agent == "pi" {
		plan.Args = []string{"auth", "check", "--model", o.Model, "--json"}
	}
	result, err := s.Runner.Execute(ctx, plan, runtime.ExecuteOptions{})
	if ctx.Err() != nil {
		return &executionCancelled{cause: ctx.Err()}
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
			return fmt.Errorf("Pi auth check for PI_CODING_AGENT_DIR=%q failed: %v; output: %s", g.Auth.Path, err, output)
		}
		provider, _, _ := strings.Cut(o.Model, "/")
		if status.Status != "ready" {
			return fmt.Errorf("Pi auth source PI_CODING_AGENT_DIR=%q not ready for provider %s: %s; no fallback identity", g.Auth.Path, status.Provider, status.Reason)
		}
		if status.Provider != provider || status.AuthType != g.Auth.Method {
			return fmt.Errorf("Pi auth source PI_CODING_AGENT_DIR=%q provides %s/%s, configured %s/%s; no fallback identity", g.Auth.Path, status.Provider, status.AuthType, provider, g.Auth.Method)
		}
		return nil
	}
	method := ""
	switch {
	case strings.Contains(output, "Logged in using ChatGPT"):
		method = "chatgpt"
	case strings.Contains(output, "Logged in using an API key"):
		method = "api-key"
	}
	if err != nil || method == "" {
		return fmt.Errorf("Codex auth source CODEX_HOME=%q is not usable: %s; no fallback identity", g.Auth.Path, output)
	}
	if method != g.Auth.Method {
		return fmt.Errorf("Codex auth source CODEX_HOME=%q uses %s login, configured %s; no fallback identity", g.Auth.Path, method, g.Auth.Method)
	}
	return nil
}
