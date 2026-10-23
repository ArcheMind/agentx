package drivers

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ArcheMind/agentx/internal/runtime"
)

// CodexPolicyExecPlan maps write scope onto Codex's native OS sandbox; no writable roots means
// read-only.
// Role instructions travel as developer_instructions, which Codex records in the rollout as a
// developer message; they never change sandbox, approval, model or effort.
func CodexPolicyExecPlan(cwd, model, resume string, writableRoots []string, effort, roleInstructions string) (runtime.CommandPlan, error) {
	args := []string{"exec"}
	if resume != "" {
		if !sessionID.MatchString(resume) {
			return runtime.CommandPlan{}, fmt.Errorf("--resume requires a Codex session UUID")
		}
		args = append(args, "resume")
	}
	args = append(args, "--json")
	if len(writableRoots) == 0 {
		args = append(args, "-c", `sandbox_mode="read-only"`)
	} else {
		roots, err := json.Marshal(writableRoots)
		if err != nil {
			return runtime.CommandPlan{}, err
		}
		args = append(args, "-c", `sandbox_mode="workspace-write"`, "-c", "sandbox_workspace_write.writable_roots="+string(roots), "-c", "sandbox_workspace_write.network_access=false", "-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true", "-c", "sandbox_workspace_write.exclude_slash_tmp=true")
	}
	args = append(args, "-c", `approval_policy="never"`)
	if effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
	}
	if roleInstructions != "" {
		value, err := tomlString(roleInstructions)
		if err != nil {
			return runtime.CommandPlan{}, err
		}
		args = append(args, "-c", "developer_instructions="+value)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if resume != "" {
		args = append(args, resume)
	}
	args = append(args, "-")
	return runtime.CommandPlan{Executable: "codex", Args: args, Cwd: cwd}, nil
}

// tomlString encodes text as a TOML basic string; JSON string escapes without \/ are valid TOML.
func tomlString(text string) (string, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(text); err != nil {
		return "", err
	}
	return string(bytes.TrimSuffix(out.Bytes(), []byte("\n"))), nil
}
