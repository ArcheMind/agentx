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
	Command           runtime.CommandPlan
	Instruction       string
	LogoutCommand     runtime.CommandPlan
	LogoutInstruction string
	StatusCommand     runtime.CommandPlan
	ParseStatus       func(runtime.CommandResult, error) (runtime.AuthStatus, error)
}

func (d NativeAuth) PlanLogin() runtime.AuthPlan {
	return runtime.AuthPlan{Command: d.Command, Instruction: d.Instruction}
}

func (d NativeAuth) PlanLogout() runtime.AuthPlan {
	return runtime.AuthPlan{Command: d.LogoutCommand, Instruction: d.LogoutInstruction}
}

func (d NativeAuth) SupportsStatus() bool {
	return d.StatusCommand.Executable != "" && d.ParseStatus != nil
}

func (d NativeAuth) SupportsLogout() bool {
	return d.LogoutCommand.Executable != ""
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
		Supported: true,
		Providers: authProviders(native.LoggedIn, runtime.AuthProvider{
			ID: "claude", Method: native.AuthMethod, Subscription: native.SubscriptionType,
		}),
	}, nil
}

func parseCodexAuthStatus(result runtime.CommandResult, executeErr error) (runtime.AuthStatus, error) {
	output := strings.TrimSpace(stripANSI(result.Stdout + result.Stderr))
	if strings.Contains(strings.ToLower(output), "not logged in") {
		return runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{}}, nil
	}
	const prefix = "Logged in using "
	if index := strings.Index(output, prefix); index >= 0 {
		method := strings.TrimSpace(output[index+len(prefix):])
		if method != "" {
			return runtime.AuthStatus{Supported: true, Providers: []runtime.AuthProvider{{ID: "codex", Method: method}}}, nil
		}
	}
	return runtime.AuthStatus{}, statusParseError("codex", executeErr, nil)
}

var openCodeCredentialPattern = regexp.MustCompile(`(?m)^●\s+(.+?)\s+(oauth|api_key|api key)\s*$`)

func parseOpenCodeAuthStatus(result runtime.CommandResult, executeErr error) (runtime.AuthStatus, error) {
	output := stripANSI(result.Stdout + result.Stderr)
	credentials, _, ok := strings.Cut(output, "Environment")
	if !ok {
		return runtime.AuthStatus{}, statusParseError("opencode", executeErr, nil)
	}
	matches := openCodeCredentialPattern.FindAllStringSubmatch(credentials, -1)
	providers := make([]runtime.AuthProvider, 0, len(matches))
	for _, match := range matches {
		providers = append(providers, runtime.AuthProvider{ID: strings.TrimSpace(match[1]), Method: strings.ReplaceAll(match[2], "_", " ")})
	}
	countPattern := regexp.MustCompile(`(?m)(\d+) credentials?\s*$`)
	match := countPattern.FindStringSubmatch(credentials)
	if len(match) != 2 {
		return runtime.AuthStatus{}, statusParseError("opencode", executeErr, nil)
	}
	count, err := strconv.Atoi(match[1])
	if err != nil || count != len(providers) {
		return runtime.AuthStatus{}, statusParseError("opencode", executeErr, err)
	}
	return runtime.AuthStatus{Supported: true, Providers: providers}, nil
}

type ProviderAuth struct {
	NativeAuth
	Providers LoggedInProviderSource
}

func (d ProviderAuth) SupportsStatus() bool { return d.Providers != nil }

func (d ProviderAuth) Status(ctx context.Context, _ runtime.Runner) (runtime.AuthStatus, error) {
	ids, err := d.Providers.ListLoggedInProviders(ctx)
	if err != nil {
		return runtime.AuthStatus{}, err
	}
	providers := make([]runtime.AuthProvider, 0, len(ids))
	for _, id := range ids {
		providers = append(providers, runtime.AuthProvider{ID: id})
	}
	return runtime.AuthStatus{Supported: true, Providers: providers}, nil
}

func authProviders(loggedIn bool, provider runtime.AuthProvider) []runtime.AuthProvider {
	if !loggedIn {
		return []runtime.AuthProvider{}
	}
	return []runtime.AuthProvider{provider}
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
