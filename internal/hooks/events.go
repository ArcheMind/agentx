package hooks

import (
	"encoding/json"
	"fmt"
)

type Name string

const (
	SessionStart Name = "session.start"
	SessionEnd   Name = "session.end"
	TurnBefore   Name = "turn.before"
	TurnAfter    Name = "turn.after"
)

type Event struct {
	Name                 Name   `json:"event" yaml:"event"`
	SessionID            string `json:"session_id" yaml:"session_id"`
	TurnID               string `json:"turn_id,omitempty" yaml:"turn_id,omitempty"`
	CWD                  string `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Source               string `json:"source,omitempty" yaml:"source,omitempty"`
	Reason               string `json:"reason,omitempty" yaml:"reason,omitempty"`
	Prompt               string `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Outcome              string `json:"outcome,omitempty" yaml:"outcome,omitempty"`
	TranscriptPath       string `json:"transcript_path,omitempty" yaml:"transcript_path,omitempty"`
	ParentSessionID      string `json:"parent_session_id,omitempty" yaml:"parent_session_id,omitempty"`
	Model                string `json:"model,omitempty" yaml:"model,omitempty"`
	Agent                string `json:"agent,omitempty" yaml:"agent,omitempty"`
	PermissionMode       string `json:"permission_mode,omitempty" yaml:"permission_mode,omitempty"`
	LastAssistantMessage string `json:"last_assistant_message,omitempty" yaml:"last_assistant_message,omitempty"`
	Error                string `json:"error,omitempty" yaml:"error,omitempty"`
	StartedAt            string `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	EndedAt              string `json:"ended_at,omitempty" yaml:"ended_at,omitempty"`
	DurationMS           int64  `json:"duration_ms,omitempty" yaml:"duration_ms,omitempty"`
}

type nativeEvent struct {
	HookEventName        string `json:"hook_event_name"`
	SessionID            string `json:"session_id"`
	TurnID               string `json:"turn_id"`
	CWD                  string `json:"cwd"`
	Source               string `json:"source"`
	Reason               string `json:"reason"`
	Prompt               string `json:"prompt"`
	Text                 string `json:"text"`
	Outcome              string `json:"outcome"`
	TranscriptPath       string `json:"transcript_path"`
	ParentSessionID      string `json:"parent_session_id"`
	Model                string `json:"model"`
	Agent                string `json:"agent"`
	AgentType            string `json:"agent_type"`
	PermissionMode       string `json:"permission_mode"`
	LastAssistantMessage string `json:"last_assistant_message"`
	PromptResponse       string `json:"prompt_response"`
	Error                string `json:"error"`
	StartedAt            string `json:"started_at"`
	EndedAt              string `json:"ended_at"`
	DurationMS           int64  `json:"duration_ms"`
}

func Decode(provider, event string, payload []byte) (Event, error) {
	var native nativeEvent
	if err := json.Unmarshal(payload, &native); err != nil {
		return Event{}, fmt.Errorf("decode %s %s hook input: %w", provider, event, err)
	}

	name, err := eventName(provider, event)
	if err != nil {
		return Event{}, err
	}
	if native.HookEventName != "" && native.HookEventName != event {
		return Event{}, fmt.Errorf("%s hook input names event %q, expected %q", provider, native.HookEventName, event)
	}
	result := Event{
		Name: name, SessionID: native.SessionID, TurnID: native.TurnID, CWD: native.CWD,
		TranscriptPath: native.TranscriptPath, ParentSessionID: native.ParentSessionID,
		Model: native.Model, Agent: native.Agent, PermissionMode: native.PermissionMode,
		Error: native.Error, StartedAt: native.StartedAt, EndedAt: native.EndedAt,
		DurationMS: native.DurationMS,
	}
	if result.Agent == "" {
		result.Agent = native.AgentType
	}
	switch name {
	case SessionStart:
		result.Source = normalizeSource(provider, native.Source, native.Reason)
	case SessionEnd:
		result.Reason = normalizeEndReason(provider, native.Reason)
	case TurnBefore:
		result.Prompt = native.Prompt
		if result.Prompt == "" && provider == "pi" {
			result.Prompt = native.Text
		}
	case TurnAfter:
		result.Outcome = normalizeOutcome(event, native.Outcome, native.Error)
		result.LastAssistantMessage = native.LastAssistantMessage
		if result.LastAssistantMessage == "" && provider == "gemini" {
			result.LastAssistantMessage = native.PromptResponse
		}
	}
	return result, nil
}

func eventName(provider, event string) (Name, error) {
	events := map[string]map[string]Name{
		"claude": {"SessionStart": SessionStart, "SessionEnd": SessionEnd, "UserPromptSubmit": TurnBefore, "Stop": TurnAfter, "StopFailure": TurnAfter},
		"codex":  {"SessionStart": SessionStart, "SessionEnd": SessionEnd, "UserPromptSubmit": TurnBefore, "Stop": TurnAfter, "Interrupt": TurnAfter},
		"gemini": {"SessionStart": SessionStart, "SessionEnd": SessionEnd, "BeforeAgent": TurnBefore, "AfterAgent": TurnAfter},
		"pi":     {"session_start": SessionStart, "session_shutdown": SessionEnd, "input": TurnBefore, "agent_settled": TurnAfter},
	}
	providerEvents, ok := events[provider]
	if !ok {
		return "", fmt.Errorf("unsupported hook provider %q", provider)
	}
	name, ok := providerEvents[event]
	if !ok {
		return "", fmt.Errorf("unsupported %s hook event %q", provider, event)
	}
	return name, nil
}

func normalizeSource(provider, source, reason string) string {
	if provider == "pi" {
		source = reason
	}
	switch source {
	case "startup", "resume", "clear", "fork":
		return source
	case "new":
		return "clear"
	default:
		return "other"
	}
}

func normalizeEndReason(provider, reason string) string {
	switch reason {
	case "completed":
		return "completed"
	case "exit", "quit", "prompt_input_exit":
		return "completed"
	case "clear", "cleared", "new":
		return "cleared"
	case "switched", "resume", "fork":
		return "switched"
	case "interrupt", "interrupted":
		return "interrupted"
	case "error":
		return "error"
	default:
		return "other"
	}
}

func normalizeOutcome(event, outcome, message string) string {
	if outcome != "" {
		return outcome
	}
	if message != "" || event == "StopFailure" {
		return "error"
	}
	if event == "Interrupt" {
		return "interrupted"
	}
	return "completed"
}
