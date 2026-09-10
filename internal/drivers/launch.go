package drivers

import (
	"fmt"

	"agentx/internal/runtime"
)

type NativeLaunch struct {
	Binary    string
	ModelFlag string
}

func (d NativeLaunch) PlanRun(request runtime.RunRequest) (runtime.CommandPlan, error) {
	args := make([]string, 0, len(request.PassthroughArgs)+2)
	if request.Model != "" {
		if d.ModelFlag == "" {
			return runtime.CommandPlan{}, fmt.Errorf("agent does not support model selection")
		}
		args = append(args, d.ModelFlag, request.Model)
	}
	args = append(args, request.PassthroughArgs...)
	return runtime.CommandPlan{Executable: d.Binary, Args: args, Cwd: request.Cwd}, nil
}
