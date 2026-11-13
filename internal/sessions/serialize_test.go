package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSerializeRoundTrip(t *testing.T) {
	t.Run("claude", testSerializeClaudeRoundTrip)
	t.Run("codex", testSerializeCodexRoundTrip)
	t.Run("pi", testSerializePiRoundTrip)
	t.Run("opencode", testSerializeOpenCodeRoundTrip)
}

func testSerializeClaudeRoundTrip(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	os.MkdirAll(workspace, 0o755)

	writeFixture(t, filepath.Join(home, ".claude", "projects", "p", "session.jsonl"), strings.Join([]string{
		fmt.Sprintf(`{"type":"user","sessionId":"s1","cwd":%q,"timestamp":"2026-09-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":"run ls"}}`, workspace),
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-09-01T10:01:00Z","uuid":"u2","parentUuid":"u1","message":{"role":"assistant","model":"claude-4","stop_reason":"tool_use","content":[{"type":"thinking","thinking":"let me think"},{"type":"text","text":"calling ls"},{"type":"tool_use","id":"tu-1","name":"bash","input":{"command":"ls"}}]}}`,
		`{"type":"user","sessionId":"s1","timestamp":"2026-09-01T10:02:00Z","uuid":"u3","parentUuid":"u2","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":"file1.txt\nfile2.txt"}]}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-09-01T10:03:00Z","uuid":"u4","parentUuid":"u3","message":{"role":"assistant","model":"claude-4","stop_reason":"end_turn","content":[{"type":"text","text":"done listing"}]}}`,
	}, "\n"))

	detail := parseAndSerializeRoundTrip(t, home, "s1", "claude", func(detail Session) string {
		records := SerializeClaude(detail)
		return marshalJSONL(t, records)
	}, func(home2 string, content string) {
		writeFixture(t, filepath.Join(home2, ".claude", "projects", "p", "session.jsonl"), content)
	})

	assertMessageRoles(t, detail.Messages, []string{"user", "assistant", "tool", "assistant"})
	assertBlockTypes(t, detail.Messages[1].Content, []string{"thinking", "text", "tool_use"})
	if detail.Messages[1].Content[0].Text != "let me think" {
		t.Fatalf("thinking text = %q", detail.Messages[1].Content[0].Text)
	}
	if detail.Messages[1].Content[2].Name != "bash" {
		t.Fatalf("tool_use name = %q", detail.Messages[1].Content[2].Name)
	}
	if detail.Messages[2].ToolUseID != "tu-1" || detail.Messages[2].ToolName != "bash" {
		t.Fatalf("tool message: useID=%q name=%q", detail.Messages[2].ToolUseID, detail.Messages[2].ToolName)
	}
	if detail.Messages[1].Model != "claude-4" {
		t.Fatalf("model = %q", detail.Messages[1].Model)
	}
	if detail.Messages[1].StopReason != "tool_use" {
		t.Fatalf("stopReason = %q", detail.Messages[1].StopReason)
	}
}

func testSerializeCodexRoundTrip(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	os.MkdirAll(workspace, 0o755)

	writeFixture(t, filepath.Join(home, ".codex", "sessions", "2026", "09", "01", "session.jsonl"), strings.Join([]string{
		fmt.Sprintf(`{"type":"session_meta","timestamp":"2026-09-01T11:00:00Z","payload":{"id":"cx1","cwd":%q,"timestamp":"2026-09-01T11:00:00Z"}}`, workspace),
		`{"type":"response_item","timestamp":"2026-09-01T11:01:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"run ls"}]}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:02:00Z","payload":{"type":"message","role":"assistant","id":"a1","content":[{"type":"output_text","text":"checking"}]}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:03:00Z","payload":{"type":"function_call","call_id":"fc-1","name":"shell","arguments":"{\"cmd\":\"ls\"}"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:04:00Z","payload":{"type":"function_call_output","call_id":"fc-1","output":"file1.txt"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:05:00Z","payload":{"type":"message","role":"assistant","id":"a2","content":[{"type":"output_text","text":"done"}]}}`,
	}, "\n"))

	detail := parseAndSerializeRoundTrip(t, home, "cx1", "codex", func(detail Session) string {
		records := SerializeCodex(detail)
		return marshalJSONL(t, records)
	}, func(home2 string, content string) {
		writeFixture(t, filepath.Join(home2, ".codex", "sessions", "2026", "09", "01", "session.jsonl"), content)
	})

	assertMessageRoles(t, detail.Messages, []string{"user", "assistant", "tool", "assistant"})
	// The assistant message should have text + tool_use blocks
	foundToolUse := false
	for _, b := range detail.Messages[1].Content {
		if b.Type == "tool_use" && b.ID == "fc-1" && b.Name == "shell" {
			foundToolUse = true
		}
	}
	if !foundToolUse {
		t.Fatal("missing tool_use block after round-trip")
	}
	if detail.Messages[2].ToolUseID != "fc-1" || detail.Messages[2].ToolName != "shell" {
		t.Fatalf("tool message: useID=%q name=%q", detail.Messages[2].ToolUseID, detail.Messages[2].ToolName)
	}
	if resultText(detail.Messages[2].Result) != "file1.txt" {
		t.Fatalf("tool output = %q", resultText(detail.Messages[2].Result))
	}
}

func testSerializePiRoundTrip(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	os.MkdirAll(workspace, 0o755)

	writeFixture(t, filepath.Join(home, ".pi", "agent", "sessions", "p", "session.jsonl"), strings.Join([]string{
		fmt.Sprintf(`{"type":"session","id":"pi1","cwd":%q,"timestamp":"2026-09-01T14:00:00Z"}`, workspace),
		`{"type":"message","id":"m0","parentId":null,"timestamp":"2026-09-01T14:00:00Z","message":{"role":"system","timestamp":1788271200000,"content":"be helpful"}}`,
		`{"type":"message","id":"m1","parentId":"m0","timestamp":"2026-09-01T14:00:00Z","message":{"role":"user","timestamp":1788271200000,"content":[{"type":"text","text":"run ls"}]}}`,
		`{"type":"message","id":"m2","parentId":"m1","timestamp":"2026-09-01T14:01:00Z","message":{"role":"assistant","timestamp":1788271260000,"stopReason":"toolUse","content":[{"type":"thinking","thinking":"reasoning","thinkingSignature":"sig-abc"},{"type":"text","text":"calling"},{"type":"toolCall","id":"pc-1","name":"shell","arguments":{"cmd":"ls"}}]}}`,
		`{"type":"message","id":"m3","parentId":"m2","timestamp":"2026-09-01T14:02:00Z","message":{"role":"toolResult","timestamp":1788271320000,"toolCallId":"pc-1","toolName":"shell","content":[{"type":"text","text":"file1.txt"}],"isError":false}}`,
		`{"type":"message","id":"m4","parentId":"m3","timestamp":"2026-09-01T14:03:00Z","message":{"role":"assistant","timestamp":1788271380000,"stopReason":"stop","content":[{"type":"text","text":"done"}]}}`,
	}, "\n"))

	detail := parseAndSerializeRoundTrip(t, home, "pi1", "pi", func(detail Session) string {
		records := SerializePi(detail)
		return marshalJSONL(t, records)
	}, func(home2 string, content string) {
		writeFixture(t, filepath.Join(home2, ".pi", "agent", "sessions", "p", "session.jsonl"), content)
	})

	assertMessageRoles(t, detail.Messages, []string{"system", "user", "assistant", "tool", "assistant"})
	if textContent(detail.Messages[0].Content) != "be helpful" {
		t.Fatalf("system text = %q", textContent(detail.Messages[0].Content))
	}
	if detail.Messages[1].ParentID != "m0" {
		t.Fatalf("user parentId = %q", detail.Messages[1].ParentID)
	}
	assertBlockTypes(t, detail.Messages[2].Content, []string{"thinking", "text", "tool_use"})
	if detail.Messages[2].Content[0].Text != "reasoning" {
		t.Fatalf("thinking text = %q", detail.Messages[2].Content[0].Text)
	}
	if detail.Messages[2].Content[0].Signature != "sig-abc" {
		t.Fatalf("thinking signature = %q", detail.Messages[2].Content[0].Signature)
	}
	if detail.Messages[3].ToolUseID != "pc-1" || detail.Messages[3].ToolName != "shell" {
		t.Fatalf("tool message: useID=%q name=%q", detail.Messages[3].ToolUseID, detail.Messages[3].ToolName)
	}
}

func testSerializeOpenCodeRoundTrip(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	os.MkdirAll(workspace, 0o755)

	root := filepath.Join(home, ".local", "share", "opencode", "project", "project", "storage", "session")
	writeFixture(t, filepath.Join(root, "info", "oc1.json"),
		fmt.Sprintf(`{"id":"oc1","title":"fix it","time":{"created":"2026-09-01T15:00:00Z","updated":"2026-09-01T15:03:00Z"}}`))
	writeFixture(t, filepath.Join(root, "message", "oc1", "msg-user.json"),
		fmt.Sprintf(`{"id":"msg-user","sessionID":"oc1","role":"user","path":{"root":%q},"time":{"created":"2026-09-01T15:00:00Z"}}`, workspace))
	writeFixture(t, filepath.Join(root, "part", "oc1", "msg-user", "p1.json"),
		`{"id":"p1","messageID":"msg-user","sessionID":"oc1","type":"text","text":"fix the bug"}`)
	writeFixture(t, filepath.Join(root, "message", "oc1", "msg-asst.json"),
		fmt.Sprintf(`{"id":"msg-asst","sessionID":"oc1","role":"assistant","path":{"root":%q},"time":{"created":"2026-09-01T15:01:00Z"}}`, workspace))
	writeFixture(t, filepath.Join(root, "part", "oc1", "msg-asst", "p2.json"),
		`{"id":"p2","messageID":"msg-asst","sessionID":"oc1","type":"text","text":"fixing"}`)
	writeFixture(t, filepath.Join(root, "part", "oc1", "msg-asst", "step-1.json"),
		`{"id":"step-1","messageID":"msg-asst","sessionID":"oc1","type":"step-start"}`)
	writeFixture(t, filepath.Join(root, "part", "oc1", "msg-asst", "step-1-finish.json"),
		`{"id":"step-1","messageID":"msg-asst","sessionID":"oc1","type":"step-finish","tokens":{"input":100,"output":50}}`)

	service := Service{HomeDir: home}
	original, err := service.Info("oc1", "opencode")
	if err != nil {
		t.Fatal(err)
	}

	info, entries := SerializeOpenCode(original, "")

	home2 := t.TempDir()
	root2 := filepath.Join(home2, ".local", "share", "opencode", "project", "project", "storage", "session")

	infoData, _ := json.Marshal(info)
	writeFixture(t, filepath.Join(root2, "info", original.ID+".json"), string(infoData))
	for _, entry := range entries {
		data, _ := json.Marshal(entry.Content)
		writeFixture(t, filepath.Join(root2, entry.Path), string(data))
	}

	service2 := Service{HomeDir: home2}
	roundTripped, err := service2.Info(original.ID, "opencode")
	if err != nil {
		t.Fatal(err)
	}

	assertMessagesEquivalent(t, original.Messages, roundTripped.Messages)

	// Verify step tool call survived
	foundStep := false
	foundStepResult := false
	for _, m := range roundTripped.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" && b.Name == "step" {
				foundStep = true
			}
		}
		if m.Role == "tool" && m.ToolName == "step" {
			foundStepResult = true
		}
	}
	if !foundStep {
		t.Fatal("missing step tool_use block after opencode round-trip")
	}
	if !foundStepResult {
		t.Fatal("missing step tool result after opencode round-trip")
	}
}

// parseAndSerializeRoundTrip parses a native fixture, serializes it back,
// writes the result to a new home dir, parses again, and verifies equivalence.
func parseAndSerializeRoundTrip(
	t *testing.T,
	home, id, agent string,
	serialize func(Session) string,
	writeBack func(home2 string, content string),
) Session {
	t.Helper()
	service := Service{HomeDir: home}
	original, err := service.Info(id, agent)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}

	content := serialize(original)

	home2 := t.TempDir()
	writeBack(home2, content)

	service2 := Service{HomeDir: home2}
	roundTripped, err := service2.Info(id, agent)
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}

	assertMessagesEquivalent(t, original.Messages, roundTripped.Messages)
	return roundTripped
}

func marshalJSONL(t *testing.T, records []map[string]any) string {
	t.Helper()
	var lines []string
	for _, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		lines = append(lines, string(data))
	}
	return strings.Join(lines, "\n")
}

func assertMessagesEquivalent(t *testing.T, a, b []Message) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("message count: got %d, want %d\n  a roles: %v\n  b roles: %v",
			len(b), len(a), messageRoles(a), messageRoles(b))
	}
	for i := range a {
		ma, mb := a[i], b[i]
		if ma.Role != mb.Role {
			t.Fatalf("message[%d] role: got %q, want %q", i, mb.Role, ma.Role)
		}
		if ma.ToolName != mb.ToolName {
			t.Fatalf("message[%d] toolName: got %q, want %q", i, mb.ToolName, ma.ToolName)
		}
		if !reflect.DeepEqual(ma.Result, mb.Result) {
			t.Fatalf("message[%d] result: got %#v, want %#v", i, mb.Result, ma.Result)
		}
		if ma.StopReason != mb.StopReason {
			t.Fatalf("message[%d] stopReason: got %q, want %q", i, mb.StopReason, ma.StopReason)
		}
		if textContent(ma.Content) != textContent(mb.Content) {
			t.Fatalf("message[%d] text: got %q, want %q", i, textContent(mb.Content), textContent(ma.Content))
		}
		if len(ma.Content) != len(mb.Content) {
			t.Fatalf("message[%d] block count: got %d, want %d\n  a types: %v\n  b types: %v",
				i, len(mb.Content), len(ma.Content), blockTypes(ma.Content), blockTypes(mb.Content))
		}
		for j := range ma.Content {
			ca, cb := ma.Content[j], mb.Content[j]
			if ca.Type != cb.Type {
				t.Fatalf("message[%d].content[%d] type: got %q, want %q", i, j, cb.Type, ca.Type)
			}
			if ca.Text != cb.Text {
				t.Fatalf("message[%d].content[%d] text: got %q, want %q", i, j, cb.Text, ca.Text)
			}
			if ca.Signature != cb.Signature || !reflect.DeepEqual(ca.Input, cb.Input) || !reflect.DeepEqual(ca.Source, cb.Source) {
				t.Fatalf("message[%d].content[%d] signature/input/source mismatch: got %#v, want %#v", i, j, cb, ca)
			}
			if ca.Name != cb.Name {
				t.Fatalf("message[%d].content[%d] name: got %q, want %q", i, j, cb.Name, ca.Name)
			}
		}
	}
}

func assertMessageRoles(t *testing.T, messages []Message, want []string) {
	t.Helper()
	got := messageRoles(messages)
	if len(got) != len(want) {
		t.Fatalf("message roles = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("message roles = %v, want %v", got, want)
		}
	}
}

func assertBlockTypes(t *testing.T, blocks []ContentBlock, want []string) {
	t.Helper()
	got := blockTypes(blocks)
	if len(got) != len(want) {
		t.Fatalf("block types = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("block types = %v, want %v", got, want)
		}
	}
}

func messageRoles(messages []Message) []string {
	roles := make([]string, len(messages))
	for i, m := range messages {
		roles[i] = m.Role
	}
	return roles
}

func blockTypes(blocks []ContentBlock) []string {
	types := make([]string, len(blocks))
	for i, b := range blocks {
		types[i] = b.Type
	}
	return types
}

// fidelityCase round-trips one native fixture through parse -> serialize -> parse
// and hands back the first and second parse for provider-specific assertions.
func fidelityRoundTrip(t *testing.T, provider, relPath, native string, serialize func(Session) string) (Session, Session, string) {
	t.Helper()
	home := t.TempDir()
	writeFixture(t, filepath.Join(home, relPath), native)
	original, err := Service{HomeDir: home}.Info("fid", provider)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := serialize(original)
	home2 := t.TempDir()
	writeFixture(t, filepath.Join(home2, relPath), out)
	again, err := Service{HomeDir: home2}.Info("fid", provider)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	assertMessagesEquivalent(t, original.Messages, again.Messages)
	return original, again, out
}

func TestClaudeFidelityRoundTrip(t *testing.T) {
	native := strings.Join([]string{
		`{"type":"user","sessionId":"fid","cwd":"/w","timestamp":"2026-09-01T10:00:00Z","uuid":"u1","message":{"role":"user","content":[{"type":"text","text":"look"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}}`,
		`{"type":"system","subtype":"informational","level":"warning","sessionId":"fid","timestamp":"2026-09-01T10:00:30Z","uuid":"s1","parentUuid":"u1","content":"Unknown command: /mocde"}`,
		`{"type":"system","subtype":"turn_duration","sessionId":"fid","timestamp":"2026-09-01T10:00:40Z","uuid":"s2","parentUuid":"s1","durationMs":5}`,
		`{"type":"assistant","sessionId":"fid","timestamp":"2026-09-01T10:01:00Z","uuid":"a1","parentUuid":"s1","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Read","input":{"p":"x"}}]}}`,
		`{"type":"user","sessionId":"fid","timestamp":"2026-09-01T10:02:00Z","uuid":"r1","parentUuid":"a1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":[{"type":"text","text":"hello"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"BBBB"}}]}]}}`,
	}, "\n")
	original, again, _ := fidelityRoundTrip(t, "claude", ".claude/projects/p/s.jsonl", native, func(d Session) string { return marshalJSONL(t, SerializeClaude(d)) })
	assertMessageRoles(t, original.Messages, []string{"user", "system", "assistant", "tool"})
	if original.Messages[0].Content[1].Type != "image" || original.Messages[0].Content[1].Source.(map[string]any)["data"] != "AAAA" {
		t.Fatalf("image block = %#v", original.Messages[0].Content)
	}
	if textContent(original.Messages[1].Content) != "Unknown command: /mocde" {
		t.Fatalf("system text = %q", textContent(original.Messages[1].Content))
	}
	result, ok := again.Messages[3].Result.([]any)
	if !ok || len(result) != 2 || result[1].(map[string]any)["type"] != "image" {
		t.Fatalf("structured tool_result not preserved verbatim: %#v", again.Messages[3].Result)
	}
	if resultText(again.Messages[3].Result) != "hello" {
		t.Fatalf("resultText = %q", resultText(again.Messages[3].Result))
	}
}

func TestCodexFidelityRoundTrip(t *testing.T) {
	native := strings.Join([]string{
		`{"type":"session_meta","timestamp":"2026-09-01T11:00:00Z","payload":{"id":"fid","cwd":"/w","timestamp":"2026-09-01T11:00:00Z"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:00:10Z","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"state index"}]}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:01:00Z","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:02:00Z","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"plan it"}],"encrypted_content":"enc-1"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:02:01Z","payload":{"type":"reasoning","summary":[],"encrypted_content":"enc-2"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:03:00Z","payload":{"type":"custom_tool_call","call_id":"c1","name":"exec","input":"tools.exec_command({cmd:'ls'})"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:04:00Z","payload":{"type":"custom_tool_call_output","call_id":"c1","output":[{"type":"input_text","text":"Script completed\nOutput:\n"},{"type":"input_text","text":"{\"exit_code\":0}"}]}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:05:00Z","payload":{"type":"function_call","call_id":"c2","name":"send_message","arguments":"{\"to\":\"x\"}"}}`,
		`{"type":"response_item","timestamp":"2026-09-01T11:06:00Z","payload":{"type":"function_call_output","call_id":"c2","output":"sent"}}`,
	}, "\n")
	original, again, out := fidelityRoundTrip(t, "codex", ".codex/sessions/2026/09/01/s.jsonl", native, func(d Session) string { return marshalJSONL(t, SerializeCodex(d)) })
	assertMessageRoles(t, original.Messages, []string{"system", "user", "assistant", "tool", "assistant", "tool"})
	blocks := original.Messages[2].Content
	assertBlockTypes(t, blocks, []string{"thinking", "thinking", "tool_use"})
	if blocks[0].Text != "plan it" || blocks[0].Signature != "enc-1" || blocks[1].Text != "" || blocks[1].Signature != "enc-2" {
		t.Fatalf("thinking blocks = %#v", blocks[:2])
	}
	if raw := blocks[2].Input.(map[string]any)["raw"]; raw != "tools.exec_command({cmd:'ls'})" {
		t.Fatalf("custom tool input = %#v", blocks[2].Input)
	}
	wantOut := []any{
		map[string]any{"type": "input_text", "text": "Script completed\nOutput:\n"},
		map[string]any{"type": "input_text", "text": "{\"exit_code\":0}"},
	}
	if !reflect.DeepEqual(original.Messages[3].Result, wantOut) || !reflect.DeepEqual(again.Messages[3].Result, wantOut) {
		t.Fatalf("array output not verbatim: %#v / %#v", original.Messages[3].Result, again.Messages[3].Result)
	}
	if again.Messages[0].Role != "system" || textContent(again.Messages[0].Content) != "state index" {
		t.Fatalf("developer message lost: %#v", again.Messages[0])
	}
	types := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		var rec struct {
			Payload struct {
				Type, Role, Input, Arguments string
			}
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		types[rec.Payload.Type]++
		if rec.Payload.Role == "developer" {
			types["developer"]++
		}
		if rec.Payload.Type == "custom_tool_call" && rec.Payload.Input != "tools.exec_command({cmd:'ls'})" {
			t.Fatalf("custom input rewritten: %q", rec.Payload.Input)
		}
	}
	for typ, want := range map[string]int{"custom_tool_call": 1, "custom_tool_call_output": 1, "function_call": 1, "function_call_output": 1, "reasoning": 2, "developer": 1} {
		if types[typ] != want {
			t.Fatalf("record type %s count = %d, want %d (all: %v)", typ, types[typ], want, types)
		}
	}
}

func TestSerializeCodexThinkingChannels(t *testing.T) {
	detail := Session{ID: "cx-think", Provider: "codex", Workspace: "/w", Messages: []Message{
		{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "go"}}, Timestamp: "2026-09-01T11:00:00Z"},
		{ID: "a1", ParentID: "u1", Role: "assistant", Timestamp: "2026-09-01T11:01:00Z", Content: []ContentBlock{
			{Type: "text", Text: "before"},
			{Type: "thinking", Text: "unsigned plan"},
			{Type: "thinking"},
			{Type: "tool_use", ID: "c1", Name: "shell", Input: map[string]any{"cmd": "ls"}},
			{Type: "thinking", Text: "signed plan", Signature: "enc-1"},
			{Type: "thinking", Text: "after signed"},
			{Type: "text", Text: "after"},
		}},
	}}
	var got []string
	for _, rec := range SerializeCodex(detail) {
		payload, _ := rec["payload"].(map[string]any)
		switch {
		case rec["type"] == "response_item" && payload["type"] == "message" && payload["role"] == "assistant":
			var texts []string
			for _, c := range payload["content"].([]any) {
				texts = append(texts, c.(map[string]any)["text"].(string))
			}
			got = append(got, "message:"+strings.Join(texts, "|"))
		case rec["type"] == "response_item" && payload["type"] == "reasoning":
			summary, _ := json.Marshal(payload["summary"])
			got = append(got, fmt.Sprintf("reasoning:%s:%v", summary, payload["encrypted_content"]))
		case rec["type"] == "response_item" && payload["type"] == "function_call":
			got = append(got, "function_call")
		case rec["type"] == "event_msg" && payload["type"] == "agent_message":
			got = append(got, "agent_message:"+payload["message"].(string))
		}
	}
	want := []string{
		"message:before|<think>\nunsigned plan\n</think>",
		"agent_message:before\n<think>\nunsigned plan\n</think>",
		"function_call",
		`reasoning:[{"text":"signed plan","type":"summary_text"}]:enc-1`,
		"message:<think>\nafter signed\n</think>|after",
		"agent_message:<think>\nafter signed\n</think>\nafter",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("codex records =\n%q\nwant\n%q", got, want)
	}
}

func TestPiFidelityRoundTrip(t *testing.T) {
	native := strings.Join([]string{
		`{"type":"session","id":"fid","cwd":"/w","timestamp":"2026-09-01T14:00:00Z"}`,
		`{"type":"message","id":"aaaa0001","parentId":null,"timestamp":"2026-09-01T14:00:00Z","message":{"role":"user","timestamp":1788271200000,"content":[{"type":"text","text":"go"}]}}`,
		`{"type":"message","id":"aaaa0002","parentId":"aaaa0001","timestamp":"2026-09-01T14:01:00.972Z","message":{"role":"assistant","timestamp":1788271260118,"stopReason":"toolUse","content":[{"type":"toolCall","id":"call|fc_1","name":"bash","arguments":{"command":"ls","n":2}}]}}`,
		`{"type":"message","id":"aaaa0003","parentId":"aaaa0002","timestamp":"2026-09-01T14:02:00Z","message":{"role":"toolResult","timestamp":1788271320000,"toolCallId":"call|fc_1","toolName":"bash","content":[{"type":"text","text":"out"}],"isError":false}}`,
		`{"type":"message","id":"aaaa0004","parentId":"aaaa0003","timestamp":"2026-09-01T14:03:00Z","message":{"role":"assistant","timestamp":1788271380000,"stopReason":"stop","content":[{"type":"text","text":"done"}]}}`,
	}, "\n")
	original, again, out := fidelityRoundTrip(t, "pi", ".pi/agent/sessions/p/s.jsonl", native, func(d Session) string { return marshalJSONL(t, SerializePi(d)) })
	ids := []string{"aaaa0001", "aaaa0002", "aaaa0003", "aaaa0004"}
	for i, m := range again.Messages {
		if m.ID != ids[i] {
			t.Fatalf("message[%d] id = %q, want %q", i, m.ID, ids[i])
		}
	}
	if original.Messages[1].StopReason != "toolUse" || original.Messages[3].StopReason != "stop" || again.Messages[3].StopReason != "stop" {
		t.Fatalf("stopReason not verbatim: %q / %q", original.Messages[1].StopReason, again.Messages[3].StopReason)
	}
	if got := again.Messages[1].Timestamp; got != "2026-09-01T14:01:00.118Z" {
		t.Fatalf("message timestamp = %q", got)
	}
	if !strings.Contains(out, `"timestamp":1788271260118`) {
		t.Fatalf("epoch millis not preserved in output: %s", out)
	}
	if again.Messages[3].ParentID != "aaaa0003" {
		t.Fatalf("parentId = %q", again.Messages[3].ParentID)
	}
	if input, ok := again.Messages[1].Content[0].Input.(map[string]any); !ok || input["command"] != "ls" {
		t.Fatalf("arguments = %#v", again.Messages[1].Content[0].Input)
	}
	var rec struct {
		Message struct {
			Content []struct {
				Arguments any `json:"arguments"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.Split(out, "\n")[2]), &rec); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.Message.Content[0].Arguments.(map[string]any); !ok {
		t.Fatalf("serialized arguments must be an object, got %#v", rec.Message.Content[0].Arguments)
	}
}

func TestSerializeUnsignedThinkingAsThinkTagText(t *testing.T) {
	detail := Session{ID: "think-tag", Workspace: "/w", Messages: []Message{
		{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "go"}}, Timestamp: "2026-09-01T11:00:00Z"},
		{ID: "a1", ParentID: "u1", Role: "assistant", Timestamp: "2026-09-01T11:01:00Z", Content: []ContentBlock{
			{Type: "thinking", Text: "unsigned plan"},
			{Type: "thinking", Text: "signed plan", Signature: "sig-1"},
			{Type: "text", Text: "after"},
		}},
	}}
	for name, test := range map[string]struct {
		serialize func(Session) []map[string]any
		sigKey    string
	}{
		"claude": {SerializeClaude, "signature"},
		"pi":     {SerializePi, "thinkingSignature"},
	} {
		detail.Provider = name
		var content []any
		for _, rec := range test.serialize(detail) {
			if msg, ok := rec["message"].(map[string]any); ok && msg["role"] == "assistant" {
				content = msg["content"].([]any)
			}
		}
		want := []any{
			map[string]any{"type": "text", "text": "<think>\nunsigned plan\n</think>"},
			map[string]any{"type": "thinking", "thinking": "signed plan", test.sigKey: "sig-1"},
			map[string]any{"type": "text", "text": "after"},
		}
		if !reflect.DeepEqual(content, want) {
			t.Fatalf("%s assistant content = %#v", name, content)
		}
	}
}

func TestUnsignedThinkingRoundTripsThroughThinkTag(t *testing.T) {
	detail := Session{ID: "3f9a1c2e-aaaa-4bbb-8ccc-123456789abc", Workspace: "/w", Messages: []Message{
		{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "go"}}, Timestamp: "2026-09-01T11:00:00Z"},
		{ID: "a1", ParentID: "u1", Role: "assistant", Timestamp: "2026-09-01T11:01:00Z", Content: []ContentBlock{
			{Type: "text", Text: "before"},
			{Type: "thinking", Text: "\nunsigned\nplan "},
			{Type: "thinking", Text: "signed plan", Signature: "sig-1"},
			{Type: "text", Text: "after"},
		}},
	}}
	want := []ContentBlock{
		{Type: "text", Text: "before"},
		{Type: "thinking", Text: "\nunsigned\nplan "},
		{Type: "thinking", Text: "signed plan", Signature: "sig-1"},
		{Type: "text", Text: "after"},
	}
	for agent, serialize := range map[string]func(Session) []map[string]any{
		"claude": SerializeClaude, "codex": SerializeCodex, "pi": SerializePi,
	} {
		detail.Provider = agent
		var records []map[string]any
		for _, line := range strings.Split(marshalJSONL(t, serialize(detail)), "\n") {
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatal(err)
			}
			records = append(records, rec)
		}
		parsed, ok := ParseRecords(agent, records)
		if !ok {
			t.Fatalf("%s did not parse back", agent)
		}
		var got []ContentBlock
		for _, m := range parsed.Messages {
			if m.Role == "assistant" {
				got = append(got, m.Content...)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s assistant blocks =\n%#v\nwant\n%#v", agent, got, want)
		}
	}
}

func TestLiftThinkTagsOnlyWholeBlocks(t *testing.T) {
	for text, want := range map[string]ContentBlock{
		"<think>\nplan\n</think>":             {Type: "thinking", Text: "plan"},
		"  <think>plan</think>\n":             {Type: "thinking", Text: "plan"},
		"<think>\n\n</think>":                 {Type: "thinking", Text: ""},
		"note <think>plan</think>":            {Type: "text", Text: "note <think>plan</think>"},
		"<think>plan</think> then answer":     {Type: "text", Text: "<think>plan</think> then answer"},
		"<think>a</think> b <think>c</think>": {Type: "text", Text: "<think>a</think> b <think>c</think>"},
		"<think>unterminated":                 {Type: "text", Text: "<think>unterminated"},
	} {
		blocks := []ContentBlock{{Type: "text", Text: text}}
		liftThinkTags(blocks)
		if !reflect.DeepEqual(blocks[0], want) {
			t.Fatalf("lift(%q) = %#v, want %#v", text, blocks[0], want)
		}
	}
}
