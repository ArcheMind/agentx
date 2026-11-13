package drivers

import (
	"fmt"

	"github.com/ArcheMind/agentx/internal/runtime"
)

// CodexPolicyExecPlan builds a non-interactive Codex invocation. Normal mode leaves sandboxing to
// the user's native Codex configuration; yolo selects danger-full-access. approval_policy="never" is
// always set because a headless run cannot answer approval prompts. userConfig=false skips user
// config.toml, exec rules and hooks. flags are appended verbatim as native CLI arguments.
func CodexPolicyExecPlan(cwd, model, resume string, yolo bool, effort string, userConfig *bool, flags []string) (runtime.CommandPlan, error) {
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
	if yolo {
		args = append(args, "-c", `sandbox_mode="danger-full-access"`)
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

// ClaudePolicyExecPlan builds a non-interactive Claude Code invocation speaking stream-json on
// both stdin and stdout, so one process spans multiple turns. Normal mode passes no permission
// flags, so Claude's own settings decide; yolo skips all permission checks.
func ClaudePolicyExecPlan(cwd, model, resume string, yolo bool, effort, roleInstructions string, flags []string) (runtime.CommandPlan, error) {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"}
	if yolo {
		args = append(args, "--dangerously-skip-permissions")
	}
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
