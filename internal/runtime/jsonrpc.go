package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

func (r ExecRunner) ExecuteJSONRPC(ctx context.Context, plan CommandPlan, requests []json.RawMessage, responseID json.RawMessage) (json.RawMessage, error) {
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, plan.Executable, plan.Args...)
	cmd.Dir = plan.Cwd
	cmd.Env = mergeEnvironment(os.Environ(), plan.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open external command %s stdin: %w", plan.Executable, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open external command %s stdout: %w", plan.Executable, err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	var input strings.Builder
	for _, request := range requests {
		input.Write(request)
		input.WriteByte('\n')
	}
	if err := cmd.Start(); err != nil {
		result := CommandResult{Stderr: stderr.String(), ExitCode: -1}
		r.log(plan, input.String(), result, err)
		return nil, fmt.Errorf("start external command %s: %w", plan.Executable, err)
	}
	if _, err := io.WriteString(stdin, input.String()); err != nil {
		cancel()
		_ = cmd.Wait()
		result := CommandResult{Stderr: stderr.String(), ExitCode: -1}
		r.log(plan, input.String(), result, err)
		return nil, fmt.Errorf("write external command %s stdin: %w", plan.Executable, err)
	}

	var output strings.Builder
	var response json.RawMessage
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		output.Write(line)
		output.WriteByte('\n')
		var envelope struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			cancel()
			_ = cmd.Wait()
			result := CommandResult{Stdout: output.String(), Stderr: stderr.String(), ExitCode: -1}
			r.log(plan, input.String(), result, err)
			return nil, fmt.Errorf("parse external command %s JSON-RPC output: %w", plan.Executable, err)
		}
		if bytes.Equal(bytes.TrimSpace(envelope.ID), bytes.TrimSpace(responseID)) {
			response = line
			break
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		cancel()
		_ = cmd.Wait()
		result := CommandResult{Stdout: output.String(), Stderr: stderr.String(), ExitCode: -1}
		r.log(plan, input.String(), result, scanErr)
		return nil, fmt.Errorf("read external command %s JSON-RPC output: %w", plan.Executable, scanErr)
	}

	_ = stdin.Close()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-time.After(time.Second):
		cancel()
		waitErr = <-waitDone
	}
	result := CommandResult{Stdout: output.String(), Stderr: stderr.String()}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
	}
	if len(response) == 0 {
		err := fmt.Errorf("external command %s closed before JSON-RPC response %s", plan.Executable, responseID)
		r.log(plan, input.String(), result, err)
		return nil, err
	}
	r.log(plan, input.String(), result, nil)
	return response, nil
}
