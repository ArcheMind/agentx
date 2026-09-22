package sessions

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNativeFormatsToUnifiedTranscript(t *testing.T) {
	for _, test := range conversionContractCases() {
		t.Run(test.provider, func(t *testing.T) {
			got := parseNativeContract(t, test.provider, test.native)
			assertSessionContract(t, got, test.unified)
		})
	}
}

func TestUnifiedTranscriptToNativeFormats(t *testing.T) {
	for _, test := range conversionContractCases() {
		t.Run(test.provider, func(t *testing.T) {
			var native string
			switch test.provider {
			case "claude":
				native = marshalJSONL(t, SerializeClaude(test.unified))
			case "codex":
				native = marshalJSONL(t, SerializeCodex(test.unified))
			case "gemini":
				data, err := json.Marshal(SerializeGemini(test.unified))
				if err != nil {
					t.Fatal(err)
				}
				native = string(data)
			case "pi":
				native = marshalJSONL(t, SerializePi(test.unified))
			}

			got := parseNativeContract(t, test.provider, native)
			assertSessionContract(t, got, test.unified)
		})
	}
}

type conversionContractCase struct {
	provider string
	native   string
	unified  Detail
}

func conversionContractCases() []conversionContractCase {
	return []conversionContractCase{
		{
			provider: "claude",
			native: strings.Join([]string{
				`{"type":"user","sessionId":"claude-contract","cwd":"/work","timestamp":"2026-09-01T10:00:00Z","uuid":"c1","message":{"role":"user","content":"run ls"}}`,
				`{"type":"assistant","sessionId":"claude-contract","timestamp":"2026-09-01T10:01:00Z","uuid":"c2","parentUuid":"c1","message":{"role":"assistant","model":"claude-4","stop_reason":"tool_use","content":[{"type":"thinking","thinking":"inspect","signature":"sig-claude"},{"type":"text","text":"checking"},{"type":"tool_use","id":"ct1","name":"shell","input":{"cmd":"ls"}}]}}`,
				`{"type":"user","sessionId":"claude-contract","timestamp":"2026-09-01T10:02:00Z","uuid":"c3","parentUuid":"c2","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"ct1","content":"file.txt","is_error":true}]}}`,
			}, "\n"),
			unified: Detail{
				ID: "claude-contract", Provider: "claude", Workspace: "/work", Title: "run ls",
				StartedAt: "2026-09-01T10:00:00Z", UpdatedAt: "2026-09-01T10:02:00Z", MessageCount: 3,
				Messages: []Message{
					{ID: "c1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "run ls"}}, Timestamp: "2026-09-01T10:00:00Z"},
					{ID: "c2", ParentID: "c1", Role: "assistant", Content: []ContentBlock{
						{Type: "thinking", Text: "inspect", Signature: "sig-claude"},
						{Type: "text", Text: "checking"},
						{Type: "tool_use", ID: "ct1", Name: "shell", Input: map[string]any{"cmd": "ls"}},
					}, Timestamp: "2026-09-01T10:01:00Z", Model: "claude-4", StopReason: "tool_use"},
					{ID: "claude-2", ParentID: "c2", Role: "tool", Content: []ContentBlock{{Type: "text", Text: "file.txt"}}, Timestamp: "2026-09-01T10:02:00Z", ToolUseID: "ct1", ToolName: "shell", IsError: true},
				},
			},
		},
		{
			provider: "codex",
			native: strings.Join([]string{
				`{"type":"session_meta","timestamp":"2026-09-01T11:00:00Z","payload":{"id":"codex-contract","cwd":"/work","timestamp":"2026-09-01T11:00:00Z","source":{"subagent":{}}}}`,
				`{"type":"response_item","timestamp":"2026-09-01T11:01:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"run ls"}]}}`,
				`{"type":"response_item","timestamp":"2026-09-01T11:02:00Z","payload":{"type":"message","role":"assistant","id":"x2","content":[{"type":"output_text","text":"checking"}]}}`,
				`{"type":"response_item","timestamp":"2026-09-01T11:03:00Z","payload":{"type":"function_call","call_id":"xt1","name":"shell","arguments":"{\"cmd\":\"ls\"}"}}`,
				`{"type":"response_item","timestamp":"2026-09-01T11:04:00Z","payload":{"type":"function_call_output","call_id":"xt1","output":"file.txt"}}`,
			}, "\n"),
			unified: Detail{
				ID: "codex-contract", Provider: "codex", IsSubagent: true, Workspace: "/work", Title: "run ls",
				StartedAt: "2026-09-01T11:00:00Z", UpdatedAt: "2026-09-01T11:04:00Z", MessageCount: 3,
				Messages: []Message{
					{ID: "codex-0", Role: "user", Content: []ContentBlock{{Type: "text", Text: "run ls"}}, Timestamp: "2026-09-01T11:01:00Z"},
					{ID: "x2", ParentID: "codex-0", Role: "assistant", Content: []ContentBlock{
						{Type: "text", Text: "checking"},
						{Type: "tool_use", ID: "xt1", Name: "shell", Input: map[string]any{"cmd": "ls"}},
					}, Timestamp: "2026-09-01T11:02:00Z"},
					{ID: "codex-2", ParentID: "x2", Role: "tool", Content: []ContentBlock{{Type: "text", Text: "file.txt"}}, Timestamp: "2026-09-01T11:04:00Z", ToolUseID: "xt1", ToolName: "shell"},
				},
			},
		},
		{
			provider: "gemini",
			native:   `{"sessionId":"gemini-contract","kind":"subagent","summary":"Inspect file","startTime":"2026-09-01T12:00:00Z","lastUpdated":"2026-09-01T12:02:00Z","messages":[{"type":"user","content":"run ls","timestamp":"2026-09-01T12:00:00Z"},{"type":"gemini","content":"checking","toolCalls":[{"id":"gt1","name":"read_file","args":{"path":"file.txt"},"status":"success","result":[{"functionResponse":{"id":"gt1","name":"read_file","response":{"output":"contents"}}}]}],"timestamp":"2026-09-01T12:01:00Z"}]}`,
			unified: Detail{
				ID: "gemini-contract", Provider: "gemini", IsSubagent: true, Title: "Inspect file",
				StartedAt: "2026-09-01T12:00:00Z", UpdatedAt: "2026-09-01T12:02:00Z", MessageCount: 3,
				Messages: []Message{
					{ID: "gemini-0", Role: "user", Content: []ContentBlock{{Type: "text", Text: "run ls"}}, Timestamp: "2026-09-01T12:00:00Z"},
					{ID: "gemini-1", ParentID: "gemini-0", Role: "assistant", Content: []ContentBlock{
						{Type: "text", Text: "checking"},
						{Type: "tool_use", ID: "gt1", Name: "read_file", Input: map[string]any{"path": "file.txt"}},
					}, Timestamp: "2026-09-01T12:01:00Z"},
					{ID: "gemini-2", ParentID: "gemini-1", Role: "tool", Content: []ContentBlock{{Type: "text", Text: "contents"}}, Timestamp: "2026-09-01T12:01:00Z", ToolUseID: "gt1", ToolName: "read_file"},
				},
			},
		},
		{
			provider: "pi",
			native: strings.Join([]string{
				`{"type":"session","id":"pi-contract","cwd":"/work","timestamp":"2026-09-01T13:00:00Z"}`,
				`{"type":"message","message":{"id":"p1","role":"user","timestamp":"2026-09-01T13:00:00Z","content":[{"type":"text","text":"run ls"}]}}`,
				`{"type":"message","message":{"id":"p2","parentId":"p1","role":"assistant","timestamp":"2026-09-01T13:01:00Z","model":"pi-model","content":[{"type":"thinking","thinking":"inspect","thinkingSignature":"sig-pi"},{"type":"text","text":"checking"},{"type":"toolCall","toolCallId":"pt1","toolName":"shell","input":{"cmd":"ls"}}]}}`,
				`{"type":"message","message":{"role":"toolResult","timestamp":"2026-09-01T13:02:00Z","toolCallId":"pt1","toolName":"shell","content":[{"type":"text","text":"file.txt"}],"isError":true}}`,
			}, "\n"),
			unified: Detail{
				ID: "pi-contract", Provider: "pi", Workspace: "/work", Title: "run ls",
				StartedAt: "2026-09-01T13:00:00Z", UpdatedAt: "2026-09-01T13:02:00Z", MessageCount: 3,
				Messages: []Message{
					{ID: "p1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "run ls"}}, Timestamp: "2026-09-01T13:00:00Z"},
					{ID: "p2", ParentID: "p1", Role: "assistant", Content: []ContentBlock{
						{Type: "thinking", Text: "inspect", Signature: "sig-pi"},
						{Type: "text", Text: "checking"},
						{Type: "tool_use", ID: "pt1", Name: "shell", Input: map[string]any{"cmd": "ls"}},
					}, Timestamp: "2026-09-01T13:01:00Z", Model: "pi-model"},
					{ID: "pi-2", ParentID: "p2", Role: "tool", Content: []ContentBlock{{Type: "text", Text: "file.txt"}}, Timestamp: "2026-09-01T13:02:00Z", ToolUseID: "pt1", ToolName: "shell", IsError: true},
				},
			},
		},
	}
}

func parseNativeContract(t *testing.T, provider, native string) Detail {
	t.Helper()
	var records []map[string]any
	if provider == "gemini" {
		var record map[string]any
		if err := json.Unmarshal([]byte(native), &record); err != nil {
			t.Fatal(err)
		}
		records = []map[string]any{record}
	} else {
		for _, line := range strings.Split(native, "\n") {
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatal(err)
			}
			records = append(records, record)
		}
	}
	detail, ok := ParseRecords(provider, records)
	if !ok {
		t.Fatalf("parse %s fixture", provider)
	}
	return detail
}

func assertSessionContract(t *testing.T, got, want Detail) {
	t.Helper()
	got.Source = ""
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unified transcript mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
