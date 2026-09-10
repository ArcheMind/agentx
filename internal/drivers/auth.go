package drivers

import "agentx/internal/runtime"

type NativeAuth struct {
	Command     runtime.CommandPlan
	Instruction string
}

func (d NativeAuth) PlanLogin() runtime.AuthPlan {
	return runtime.AuthPlan{Command: d.Command, Instruction: d.Instruction}
}
