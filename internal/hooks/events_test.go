package hooks_test

import (
	"reflect"
	"testing"

	"github.com/ArcheMind/agentx/internal/hooks"
)

func TestSessionStart(t *testing.T) {
	want := hooks.Event{Name: hooks.SessionStart, SessionID: "session-1", CWD: "/work", Source: "resume"}
	cases := []struct {
		provider string
		event    string
		payload  string
	}{
		{"claude", "SessionStart", `{"hook_event_name":"SessionStart","session_id":"session-1","cwd":"/work","source":"resume"}`},
		{"codex", "SessionStart", `{"hook_event_name":"SessionStart","session_id":"session-1","cwd":"/work","source":"resume"}`},
		{"gemini", "SessionStart", `{"hook_event_name":"SessionStart","session_id":"session-1","cwd":"/work","source":"resume"}`},
		{"pi", "session_start", `{"session_id":"session-1","cwd":"/work","reason":"resume"}`},
	}

	for _, test := range cases {
		t.Run(test.provider, func(t *testing.T) {
			got, err := hooks.Decode(test.provider, test.event, []byte(test.payload))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("event = %#v, want %#v", got, want)
			}
		})
	}
}

func TestSessionEnd(t *testing.T) {
	want := hooks.Event{Name: hooks.SessionEnd, SessionID: "session-1", CWD: "/work", Reason: "completed"}
	cases := []struct {
		provider string
		event    string
		payload  string
	}{
		{"claude", "SessionEnd", `{"hook_event_name":"SessionEnd","session_id":"session-1","cwd":"/work","reason":"exit"}`},
		{"codex", "SessionEnd", `{"hook_event_name":"SessionEnd","session_id":"session-1","cwd":"/work","reason":"completed"}`},
		{"gemini", "SessionEnd", `{"hook_event_name":"SessionEnd","session_id":"session-1","cwd":"/work","reason":"exit"}`},
		{"pi", "session_shutdown", `{"session_id":"session-1","cwd":"/work","reason":"quit"}`},
	}

	for _, test := range cases {
		t.Run(test.provider, func(t *testing.T) {
			got, err := hooks.Decode(test.provider, test.event, []byte(test.payload))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("event = %#v, want %#v", got, want)
			}
		})
	}
}

func TestTurnBefore(t *testing.T) {
	want := hooks.Event{Name: hooks.TurnBefore, SessionID: "session-1", CWD: "/work", Prompt: "review this"}
	cases := []struct {
		provider string
		event    string
		payload  string
	}{
		{"claude", "UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","session_id":"session-1","cwd":"/work","prompt":"review this"}`},
		{"codex", "UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","session_id":"session-1","cwd":"/work","prompt":"review this"}`},
		{"gemini", "BeforeAgent", `{"hook_event_name":"BeforeAgent","session_id":"session-1","cwd":"/work","prompt":"review this"}`},
		{"pi", "input", `{"session_id":"session-1","cwd":"/work","text":"review this"}`},
	}

	for _, test := range cases {
		t.Run(test.provider, func(t *testing.T) {
			got, err := hooks.Decode(test.provider, test.event, []byte(test.payload))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("event = %#v, want %#v", got, want)
			}
		})
	}
}

func TestTurnAfter(t *testing.T) {
	want := hooks.Event{Name: hooks.TurnAfter, SessionID: "session-1", CWD: "/work", Outcome: "completed", LastAssistantMessage: "done"}
	cases := []struct {
		provider string
		event    string
		payload  string
	}{
		{"claude", "Stop", `{"hook_event_name":"Stop","session_id":"session-1","cwd":"/work","last_assistant_message":"done"}`},
		{"codex", "Stop", `{"hook_event_name":"Stop","session_id":"session-1","cwd":"/work","last_assistant_message":"done"}`},
		{"gemini", "AfterAgent", `{"hook_event_name":"AfterAgent","session_id":"session-1","cwd":"/work","prompt_response":"done"}`},
		{"pi", "agent_settled", `{"session_id":"session-1","cwd":"/work","last_assistant_message":"done"}`},
	}

	for _, test := range cases {
		t.Run(test.provider, func(t *testing.T) {
			got, err := hooks.Decode(test.provider, test.event, []byte(test.payload))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("event = %#v, want %#v", got, want)
			}
		})
	}
}
