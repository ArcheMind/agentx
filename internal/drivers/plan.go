package drivers

import (
	"fmt"

	"github.com/ArcheMind/agentx/internal/agentparams"
	"github.com/ArcheMind/agentx/internal/runtime"
)

func Plan(agent string, transport agentparams.Transport, request agentparams.Request) (runtime.CommandPlan, error) {
	plan, err := agentparams.Build(agent, transport, request)
	if err != nil {
		return runtime.CommandPlan{}, err
	}
	if len(plan.Params) > 0 || len(plan.Settings) > 0 {
		return runtime.CommandPlan{}, fmt.Errorf("%s %s requires a native protocol session", agent, transport)
	}
	return runtime.CommandPlan{Executable: agent, Args: plan.Args, Cwd: request.Cwd}, nil
}
