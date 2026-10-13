package drivers

import (
	"fmt"
	"regexp"

	"github.com/ArcheMind/agentx/internal/runtime"
)

var sessionID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func CodexExecPlan(cwd, model, resume string) (runtime.CommandPlan, error) {
	args := []string{"exec"}
	if resume != "" {
		if !sessionID.MatchString(resume) {
			return runtime.CommandPlan{}, fmt.Errorf("--resume requires a Codex session UUID")
		}
		args = append(args, "resume")
	}
	args = append(args, "--json", "-c", `sandbox_mode="read-only"`, "-c", `approval_policy="never"`)
	if model != "" {
		args = append(args, "--model", model)
	}
	if resume != "" {
		args = append(args, resume)
	}
	args = append(args, "-")
	return runtime.CommandPlan{Executable: "codex", Args: args, Cwd: cwd}, nil
}
