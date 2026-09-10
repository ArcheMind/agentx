package drivers

import (
	"fmt"

	"agentx/internal/runtime"
)

type CASRSessions struct{}

func (CASRSessions) Plan(args []string) (runtime.CommandPlan, error) {
	if len(args) == 0 {
		return runtime.CommandPlan{}, fmt.Errorf("session command requires providers, list, info, or resume")
	}
	switch args[0] {
	case "providers", "list", "info", "resume":
	default:
		return runtime.CommandPlan{}, fmt.Errorf("unsupported session command %q", args[0])
	}
	return runtime.CommandPlan{Executable: "casr", Args: args}, nil
}
