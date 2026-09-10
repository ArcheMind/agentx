package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type ExecRunner struct {
	Debug bool
	Log   io.Writer
}

type externalCallLog struct {
	Event    string      `json:"event"`
	Input    CommandPlan `json:"input"`
	Output   string      `json:"output"`
	ErrorOut string      `json:"error_output"`
	Error    string      `json:"error,omitempty"`
	ExitCode int         `json:"exit_code"`
}

func (r ExecRunner) Execute(ctx context.Context, plan CommandPlan, options ExecuteOptions) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, plan.Executable, plan.Args...)
	cmd.Dir = plan.Cwd
	cmd.Env = mergeEnvironment(os.Environ(), plan.Env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if options.Interactive {
		cmd.Stdin = options.Stdin
		cmd.Stdout = options.Stdout
		cmd.Stderr = options.Stderr
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}
	r.log(plan, result, err)
	if err != nil {
		return result, fmt.Errorf("external command %s failed: %w", plan.Executable, err)
	}
	return result, nil
}

func (r ExecRunner) log(plan CommandPlan, result CommandResult, err error) {
	if !r.Debug {
		return
	}
	entry := externalCallLog{
		Event:    "external_call",
		Input:    plan,
		Output:   result.Stdout,
		ErrorOut: result.Stderr,
		ExitCode: result.ExitCode,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	encoded, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		fmt.Fprintf(r.Log, "debug log encoding failed: %v\n", marshalErr)
		return
	}
	fmt.Fprintln(r.Log, string(encoded))
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, item := range base {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}
