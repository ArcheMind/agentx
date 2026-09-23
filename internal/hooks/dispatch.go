package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	agentruntime "github.com/ArcheMind/agentx/internal/runtime"
)

type DispatchResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Handler struct {
	Command string  `json:"command"`
	Matcher string  `json:"matcher,omitempty"`
	Timeout float64 `json:"timeout"`
	Project string  `json:"project,omitempty"`
}

type commandOutput struct {
	Decision           string `json:"decision"`
	Reason             string `json:"reason"`
	SystemMessage      string `json:"systemMessage"`
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

type commandResult struct {
	index    int
	output   commandOutput
	plain    string
	stderr   string
	exitCode int
	err      error
}

func DispatchOne(ctx context.Context, runner agentruntime.Runner, provider, event string, payload []byte, handler Handler) (DispatchResult, error) {
	if !isPortableTarget(provider) {
		return DispatchResult{}, fmt.Errorf("unsupported hook provider %q", provider)
	}
	if !isPortableEvent(event) {
		return DispatchResult{}, fmt.Errorf("unsupported portable hook event %q", event)
	}
	native, err := providerEvent(provider, event)
	if err != nil {
		return DispatchResult{}, err
	}
	canonical, input, err := canonicalInput(provider, native, event, payload)
	if err != nil {
		return DispatchResult{}, err
	}
	if handler.Project != "" && filepath.Clean(handler.Project) != filepath.Clean(canonical.CWD) {
		return DispatchResult{}, nil
	}
	if !matches(handler.Matcher, matcherSubject(canonical)) {
		return DispatchResult{}, nil
	}
	return dispatchHandlers(ctx, runner, provider, event, canonical, input, []Handler{handler})
}

func dispatchHandlers(ctx context.Context, runner agentruntime.Runner, provider, event string, canonical Event, input []byte, handlers []Handler) (DispatchResult, error) {
	results := make([]commandResult, len(handlers))
	var wait sync.WaitGroup
	for index, handler := range handlers {
		wait.Add(1)
		go func(index int, handler Handler) {
			defer wait.Done()
			results[index] = runHook(ctx, runner, index, handler, canonical.CWD, input)
		}(index, handler)
	}
	wait.Wait()
	return aggregate(provider, event, results)
}

func canonicalInput(provider, native, event string, payload []byte) (Event, []byte, error) {
	decoded, err := Decode(provider, native, payload)
	if err != nil {
		return Event{}, nil, err
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		return Event{}, nil, fmt.Errorf("encode portable %s hook input: %w", event, err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return Event{}, nil, fmt.Errorf("encode portable %s hook input: %w", event, err)
	}
	delete(object, "event")
	object["hook_event_name"] = event
	data, err = json.Marshal(object)
	if err != nil {
		return Event{}, nil, fmt.Errorf("encode portable %s hook input: %w", event, err)
	}
	return decoded, data, nil
}

func runHook(ctx context.Context, runner agentruntime.Runner, index int, handler Handler, cwd string, input []byte) commandResult {
	commandCtx := ctx
	cancel := func() {}
	if handler.Timeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, time.Duration(handler.Timeout*float64(time.Second)))
	}
	defer cancel()
	plan := shellPlan(handler.Command, cwd)
	result, err := runner.Execute(commandCtx, plan, agentruntime.ExecuteOptions{Stdin: bytes.NewReader(input)})
	parsed := commandOutput{}
	plain := ""
	trimmed := strings.TrimSpace(result.Stdout)
	if trimmed != "" {
		if jsonErr := json.Unmarshal([]byte(trimmed), &parsed); jsonErr != nil {
			plain = trimmed
		}
	}
	return commandResult{index: index, output: parsed, plain: plain, stderr: strings.TrimSpace(result.Stderr), exitCode: result.ExitCode, err: err}
}

func shellPlan(command, cwd string) agentruntime.CommandPlan {
	if runtime.GOOS == "windows" {
		return agentruntime.CommandPlan{Executable: "cmd.exe", Args: []string{"/D", "/S", "/C", command}, Cwd: cwd}
	}
	return agentruntime.CommandPlan{Executable: "sh", Args: []string{"-c", command}, Cwd: cwd}
}

func aggregate(provider, event string, results []commandResult) (DispatchResult, error) {
	sort.Slice(results, func(i, j int) bool { return results[i].index < results[j].index })
	systemMessages := []string{}
	contexts := []string{}
	diagnostics := []string{}
	failures := []string{}
	blocked := false
	blockReason := ""
	for _, result := range results {
		if result.stderr != "" {
			diagnostics = append(diagnostics, result.stderr)
		}
		if result.plain != "" {
			systemMessages = append(systemMessages, result.plain)
		}
		if result.output.SystemMessage != "" {
			systemMessages = append(systemMessages, result.output.SystemMessage)
		}
		if result.output.HookSpecificOutput.AdditionalContext != "" {
			contexts = append(contexts, result.output.HookSpecificOutput.AdditionalContext)
		}
		decision := strings.ToLower(result.output.Decision)
		if event == "UserPromptSubmit" && (decision == "block" || decision == "deny" || result.exitCode == 2) {
			blocked = true
			if blockReason == "" {
				blockReason = result.output.Reason
				if blockReason == "" {
					blockReason = result.stderr
				}
			}
			continue
		}
		if result.err != nil {
			message := result.stderr
			if message == "" {
				message = result.err.Error()
			}
			failures = append(failures, message)
		}
	}
	if blocked {
		if blockReason == "" {
			blockReason = "Hook blocked the turn."
		}
		output, err := encodeOutput(provider, event, strings.Join(systemMessages, "\n"), "", "block", blockReason)
		return DispatchResult{Stdout: output, Stderr: strings.Join(diagnostics, "\n"), ExitCode: 0}, err
	}
	if len(failures) > 0 {
		return DispatchResult{Stderr: strings.Join(failures, "\n"), ExitCode: 1}, errors.New(strings.Join(failures, "; "))
	}
	contextText := ""
	if event == "SessionStart" || event == "UserPromptSubmit" {
		contextText = strings.Join(contexts, "\n")
	}
	output, err := encodeOutput(provider, event, strings.Join(systemMessages, "\n"), contextText, "", "")
	return DispatchResult{Stdout: output, Stderr: strings.Join(diagnostics, "\n")}, err
}

func encodeOutput(provider, event, systemMessage, additionalContext, decision, reason string) (string, error) {
	if systemMessage == "" && additionalContext == "" && decision == "" {
		return "", nil
	}
	output := map[string]any{}
	if systemMessage != "" {
		output["systemMessage"] = systemMessage
	}
	if decision != "" {
		nativeDecision := decision
		if provider == "gemini" && decision == "block" {
			nativeDecision = "deny"
		}
		output["decision"] = nativeDecision
		output["reason"] = reason
	}
	if additionalContext != "" {
		native, err := providerEvent(provider, event)
		if err != nil {
			return "", err
		}
		if provider == "pi" {
			native = event
		}
		output["hookSpecificOutput"] = map[string]string{"hookEventName": native, "additionalContext": additionalContext}
	}
	data, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode %s hook output: %w", provider, err)
	}
	return string(data) + "\n", nil
}

func matcherSubject(event Event) string {
	switch event.Name {
	case SessionStart:
		return event.Source
	case SessionEnd:
		return event.Reason
	default:
		return ""
	}
}

func matches(pattern, subject string) bool {
	if pattern == "" || pattern == "*" || subject == "" {
		return true
	}
	matched, err := regexp.MatchString(pattern, subject)
	return err == nil && matched
}
