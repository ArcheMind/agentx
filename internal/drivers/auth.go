package drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"agentx/internal/runtime"
)

type NativeAuth struct {
	Command       runtime.CommandPlan
	Instruction   string
	StatusCommand runtime.CommandPlan
	ParseStatus   func(runtime.CommandResult, error) (runtime.AuthStatus, error)
}

func (d NativeAuth) PlanLogin() runtime.AuthPlan {
	return runtime.AuthPlan{Command: d.Command, Instruction: d.Instruction}
}

func (d NativeAuth) SupportsStatus() bool {
	return d.StatusCommand.Executable != "" && d.ParseStatus != nil
}

func (d NativeAuth) Status(ctx context.Context, runner runtime.Runner) (runtime.AuthStatus, error) {
	if d.StatusCommand.Executable == "" || d.ParseStatus == nil {
		return runtime.AuthStatus{Supported: false}, nil
	}
	result, executeErr := runner.Execute(ctx, d.StatusCommand, runtime.ExecuteOptions{})
	return d.ParseStatus(result, executeErr)
}

func parseClaudeAuthStatus(result runtime.CommandResult, executeErr error) (runtime.AuthStatus, error) {
	var native struct {
		LoggedIn         bool   `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		SubscriptionType string `json:"subscriptionType"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &native); err != nil {
		return runtime.AuthStatus{}, statusParseError("claude", executeErr, err)
	}
	return runtime.AuthStatus{
		Supported:    true,
		LoggedIn:     native.LoggedIn,
		Method:       native.AuthMethod,
		Subscription: native.SubscriptionType,
	}, nil
}

func parseCodexAuthStatus(result runtime.CommandResult, executeErr error) (runtime.AuthStatus, error) {
	output := strings.TrimSpace(stripANSI(result.Stdout + result.Stderr))
	if strings.Contains(strings.ToLower(output), "not logged in") {
		return runtime.AuthStatus{Supported: true}, nil
	}
	const prefix = "Logged in using "
	if index := strings.Index(output, prefix); index >= 0 {
		method := strings.TrimSpace(output[index+len(prefix):])
		if method != "" {
			return runtime.AuthStatus{Supported: true, LoggedIn: true, Method: method}, nil
		}
	}
	return runtime.AuthStatus{}, statusParseError("codex", executeErr, nil)
}

var credentialCountPattern = regexp.MustCompile(`(?m)(\d+) credentials?\s*$`)

func parseOpenCodeAuthStatus(result runtime.CommandResult, executeErr error) (runtime.AuthStatus, error) {
	output := stripANSI(result.Stdout + result.Stderr)
	match := credentialCountPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return runtime.AuthStatus{}, statusParseError("opencode", executeErr, nil)
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return runtime.AuthStatus{}, statusParseError("opencode", executeErr, err)
	}
	status := runtime.AuthStatus{Supported: true, LoggedIn: count > 0}
	if count > 0 {
		status.Method = fmt.Sprintf("%d configured credential", count)
		if count != 1 {
			status.Method += "s"
		}
	}
	return status, nil
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

func statusParseError(agent string, executeErr, parseErr error) error {
	if executeErr != nil {
		return fmt.Errorf("read %s authentication status: %w", agent, executeErr)
	}
	if parseErr != nil {
		return fmt.Errorf("parse %s authentication status: %w", agent, parseErr)
	}
	return fmt.Errorf("parse %s authentication status: unrecognized native output", agent)
}
