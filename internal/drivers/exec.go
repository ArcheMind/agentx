package drivers

import (
	"fmt"
	"regexp"

	"github.com/ArcheMind/agentx/internal/runtime"
)

var sessionID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func ExecPlan(agent, cwd, model, resume string) (runtime.CommandPlan, error) {
	if agent == "codex" {
		return CodexExecPlan(cwd, model, resume)
	}
	if resume != "" && !sessionID.MatchString(resume) {
		return runtime.CommandPlan{}, fmt.Errorf("--resume requires an exact session UUID")
	}
	var args []string
	switch agent {
	case "pi":
		args = []string{"--print", "--mode", "json", "--tools", "read,grep,find,ls", "--no-extensions"}
		if resume != "" {
			args = append(args, "--session", resume)
		}
	case "gemini":
		args = []string{"--output-format", "stream-json", "--approval-mode", "plan", "--prompt", ""}
		if resume != "" {
			args = append(args, "--resume", resume)
		}
	default:
		return runtime.CommandPlan{}, fmt.Errorf("%s has no native subagent execution adapter", agent)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return runtime.CommandPlan{Executable: agent, Args: args, Cwd: cwd}, nil
}

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
