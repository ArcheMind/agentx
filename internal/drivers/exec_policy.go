package drivers

import (
	"encoding/json"
	"fmt"

	"github.com/ArcheMind/agentx/internal/runtime"
)

// CodexPolicyExecPlan maps write scope onto Codex's native OS sandbox; no writable roots means
// read-only. sandbox=false drops the network and tmpdir restrictions of the writable-roots mode.
// userConfig=false skips user config.toml, exec rules and hooks, whose MCP servers, hooks and
// notify programs run outside the sandbox. flags are appended verbatim as native CLI arguments.
func CodexPolicyExecPlan(cwd, model, resume string, writableRoots []string, effort string, sandbox *bool, userConfig *bool, flags []string) (runtime.CommandPlan, error) {
	args := []string{"exec"}
	if resume != "" {
		if !sessionID.MatchString(resume) {
			return runtime.CommandPlan{}, fmt.Errorf("--resume requires a Codex session UUID")
		}
		args = append(args, "resume")
	}
	args = append(args, "--json", "--skip-git-repo-check")
	if userConfig != nil && !*userConfig {
		args = append(args, "--ignore-user-config", "--ignore-rules", "--disable", "hooks")
	}
	if len(writableRoots) == 0 {
		args = append(args, "-c", `sandbox_mode="read-only"`)
	} else {
		roots, err := json.Marshal(writableRoots)
		if err != nil {
			return runtime.CommandPlan{}, err
		}
		args = append(args, "-c", `sandbox_mode="workspace-write"`, "-c", "sandbox_workspace_write.writable_roots="+string(roots))
		if sandbox == nil || *sandbox {
			args = append(args, "-c", "sandbox_workspace_write.network_access=false", "-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true", "-c", "sandbox_workspace_write.exclude_slash_tmp=true")
		}
	}
	args = append(args, "-c", `approval_policy="never"`)
	if effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if resume != "" {
		args = append(args, resume)
	}
	args = append(args, flags...)
	args = append(args, "-")
	return runtime.CommandPlan{Executable: "codex", Args: args, Cwd: cwd}, nil
}

// ClaudePolicyExecPlan builds a non-interactive Claude Code invocation. Claude manages its own
// permissions and authentication; AgentX passes effort, model, role instructions and flags through.
func ClaudePolicyExecPlan(cwd, model, resume string, effort, roleInstructions string, flags []string) (runtime.CommandPlan, error) {
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if resume != "" {
		if !sessionID.MatchString(resume) {
			return runtime.CommandPlan{}, fmt.Errorf("--resume requires a Claude session UUID")
		}
		args = append(args, "--resume", resume)
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	if roleInstructions != "" {
		args = append(args, "--append-system-prompt", roleInstructions)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, flags...)
	return runtime.CommandPlan{Executable: "claude", Args: args, Cwd: cwd}, nil
}
