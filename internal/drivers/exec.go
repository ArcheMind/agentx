package drivers

import (
	"fmt"
	"regexp"

	"github.com/ArcheMind/agentx/internal/runtime"
)

var sessionID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func ExecPlan(agent, cwd, model, resume string) (runtime.CommandPlan, error) {
	if resume != "" && !sessionID.MatchString(resume) {
		return runtime.CommandPlan{}, fmt.Errorf("--resume requires an exact session UUID")
	}
	var args []string
	switch agent {
	case "agy":
		args = []string{"--input-format", "stream-json", "--output-format", "stream-json"}
		if resume != "" {
			args = append(args, "--conversation", resume)
		}
	case "pi":
		args = []string{"--print", "--mode", "json"}
		if resume != "" {
			args = append(args, "--session", resume)
		}
	default:
		return runtime.CommandPlan{}, fmt.Errorf("%s has no native subagent execution adapter", agent)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return runtime.CommandPlan{Executable: agent, Args: args, Cwd: cwd}, nil
}
