package subagents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/presets"
)

// Grant is the user's execution authorization for one preset. It is keyed by preset name and kept
// outside preset identity so presets stay free of permission and credential material.
type Grant struct {
	Execution ExecutionPolicy `json:"execution"`
	Auth      AuthSource      `json:"auth"`
}

type ExecutionPolicy struct {
	Policy        string   `json:"policy"`
	WritableRoots []string `json:"writable_roots,omitempty"`
	Approval      string   `json:"approval"`
}

type AuthSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	Method string `json:"method"`
}

var authSources = map[string]struct {
	source  string
	methods []string
}{
	"codex": {"codex-home", []string{"chatgpt", "api-key"}},
	"pi":    {"pi-agent-dir", []string{"oauth", "api_key"}},
}

// Agent is the execution agent implied by the auth source, or "" for an unsupported source.
func (g Grant) Agent() string {
	for name, expected := range authSources {
		if expected.source == g.Auth.Source {
			return name
		}
	}
	return ""
}

// Validate checks the grant's own values against the agent its auth source implies; whether that
// agent matches a preset is a per-preset readiness question (Config.Ready), not a structural one.
func (g Grant) Validate() error {
	agent := g.Agent()
	if agent == "" {
		return fmt.Errorf("unsupported auth.source %q; supported: codex-home, pi-agent-dir", g.Auth.Source)
	}
	p := g.Execution
	switch p.Policy {
	case drivers.PolicyReadOnly:
		if len(p.WritableRoots) != 0 {
			return fmt.Errorf("read-only policy must not list writable_roots")
		}
	case drivers.PolicyBoundedWrite:
		if agent == "pi" {
			return fmt.Errorf("bounded-write is unsupported for Pi: Pi has no runtime sandbox and its tool allowlist cannot bound bash/edit/write side effects")
		}
		if len(p.WritableRoots) == 0 {
			return fmt.Errorf("bounded-write requires at least one writable_roots entry")
		}
		for _, root := range p.WritableRoots {
			if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
				return fmt.Errorf("writable root %q must be an absolute, clean, non-root path", root)
			}
		}
	case "":
		return fmt.Errorf("execution.policy is required; supported: read-only, bounded-write")
	default:
		return fmt.Errorf("unsupported execution policy %q; supported: read-only, bounded-write", p.Policy)
	}
	if p.Approval != "never" {
		return fmt.Errorf("unsupported approval %q; supported: never (non-interactive execution has no human approval path and does not inherit Claude approvals)", p.Approval)
	}
	expected := authSources[agent]
	if !filepath.IsAbs(g.Auth.Path) || filepath.Clean(g.Auth.Path) != g.Auth.Path {
		return fmt.Errorf("auth.path %q must be an absolute, clean path", g.Auth.Path)
	}
	if !presets.Contains(expected.methods, g.Auth.Method) {
		return fmt.Errorf("auth.method %q unsupported for %s; supported: %s", g.Auth.Method, agent, strings.Join(expected.methods, ", "))
	}
	return nil
}

func (g Grant) Ready() error {
	for _, dir := range append([]string{g.Auth.Path}, g.Execution.WritableRoots...) {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("configured directory unavailable: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("configured path %s is not a directory", dir)
		}
	}
	return nil
}

// ResolvedRoots returns symlink-resolved writable roots and rejects a working directory outside
// them, because Codex always makes its working directory writable under workspace-write.
func (g Grant) ResolvedRoots(cwd string) ([]string, error) {
	if g.Execution.Policy != drivers.PolicyBoundedWrite {
		return nil, nil
	}
	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, err
	}
	roots := []string{}
	inside := false
	for _, root := range g.Execution.WritableRoots {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, fmt.Errorf("writable root unavailable: %w", err)
		}
		roots = append(roots, real)
		if rel, err := filepath.Rel(real, realCwd); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			inside = true
		}
	}
	if !inside {
		return nil, fmt.Errorf("working directory %s is outside writable roots %v; Codex would make it writable, exceeding the grant; no task started", cwd, g.Execution.WritableRoots)
	}
	return roots, nil
}

func (g Grant) Summary() string {
	scope := g.Execution.Policy
	if len(g.Execution.WritableRoots) > 0 {
		scope += fmt.Sprintf(" roots=%q", g.Execution.WritableRoots)
	}
	return fmt.Sprintf("policy=%s; approval=%s; auth=%s %q (%s)", scope, g.Execution.Approval, g.Auth.Source, g.Auth.Path, g.Auth.Method)
}

func Limits(o presets.Preset, g Grant) string {
	if !presets.Agents[o.Agent].Subagent {
		return "Not callable as ExternalSubagent; usable for interactive ax launch only."
	}
	if g.Execution.Policy == "" {
		return "Not callable as ExternalSubagent until a grant (execution policy, approval and auth source) is configured; nothing runs without one."
	}
	var text strings.Builder
	if o.Agent == "pi" {
		text.WriteString("Execution policy: read-only, enforced only by Pi's tool allowlist (read, grep, find, ls; no bash, edit or write); this is an application-level tool filter, not an OS sandbox; reads are not path-restricted. Extensions, skills, prompt templates and project approval disabled. Bounded-write is unavailable for Pi (no runtime sandbox). ")
		text.WriteString("Approval: never (user-selected; Pi has no approval path; Claude permissions and approvals are not inherited). ")
		fmt.Fprintf(&text, "Auth: PI_CODING_AGENT_DIR=%q expecting %s for the model's provider, verified by pi auth check before each execution. ", g.Auth.Path, g.Auth.Method)
	} else {
		if g.Execution.Policy == drivers.PolicyBoundedWrite {
			fmt.Fprintf(&text, "Execution policy: bounded-write, enforced by Codex native OS sandbox (workspace-write): writable only inside %q (working directory must be inside one; .git inside them stays read-only); command writes elsewhere, including /tmp and $TMPDIR, and network access from commands are blocked by the sandbox; apply_patch outside the roots is rejected by Codex application-level patch approval; reads are not path-restricted. ", g.Execution.WritableRoots)
		} else {
			text.WriteString("Execution policy: read-only, enforced by Codex native OS sandbox: no file writes, network access from commands blocked; reads are not path-restricted. ")
		}
		text.WriteString("Approval: never (user-selected; sandbox denials are returned to the agent; no human approval path; Claude permissions and approvals are not inherited). Codex user config.toml, exec rules and hooks are not loaded (their MCP servers, hooks and notify programs would run outside the sandbox); Codex built-in web search remains available. ")
		fmt.Fprintf(&text, "Auth: CODEX_HOME=%q expecting %s login, verified by codex login status before each execution. ", g.Auth.Path, g.Auth.Method)
	}
	text.WriteString("Auth identity means the configured source directory plus login method; the account inside that source is not verified. Inherited credential-like environment variables are not passed; no fallback identity. ")
	text.WriteString("Role: general-purpose only. ")
	text.WriteString(IsolationLimits(o.Agent))
	if o.Agent == "pi" {
		text.WriteString("Effort: native default only (Pi may silently clamp thinking levels). Exact provider/model required. ")
	} else {
		text.WriteString("Effort is fixed by the preset; native rejection of an unsupported level is returned as failure. ")
	}
	text.WriteString("Remote availability is unverified until execution.")
	return text.String()
}
