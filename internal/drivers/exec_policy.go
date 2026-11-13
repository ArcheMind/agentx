package drivers

import (
	"fmt"

	"github.com/ArcheMind/agentx/internal/runtime"
)

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
