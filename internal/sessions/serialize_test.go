package sessions

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSerializeRoundTrip(t *testing.T) {
	t.Run("claude", testSerializeClaudeRoundTrip)
	t.Run("codex", testSerializeCodexRoundTrip)
	t.Run("gemini", testSerializeGeminiRoundTrip)
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

	detail := parseAndSerializeRoundTrip(t, home, "s1", "claude", func(detail Detail) string {
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

	detail := parseAndSerializeRoundTrip(t, home, "cx1", "codex", func(detail Detail) string {
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
	if textContent(detail.Messages[2].Content) != "file1.txt" {
		t.Fatalf("tool output = %q", textContent(detail.Messages[2].Content))
	}
}

func testSerializeGeminiRoundTrip(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	os.MkdirAll(workspace, 0o755)

	projectHash := fmt.Sprintf("%x", sha256.Sum256([]byte(workspace)))
	geminiDir := filepath.Join(home, ".gemini", "tmp", projectHash)
	writeFixture(t, filepath.Join(geminiDir, ".project_root"), workspace)
	writeFixture(t, filepath.Join(geminiDir, "chats", "session-g1.json"),
		`{"sessionId":"g1","startTime":"2026-09-01T12:00:00Z","lastUpdated":"2026-09-01T12:03:00Z","messages":[{"type":"user","content":"run ls","timestamp":"2026-09-01T12:00:00Z"},{"type":"gemini","content":"checking","toolCalls":[{"id":"tc-1","name":"read_file","args":{"path":"a.txt"},"status":"success","result":[{"functionResponse":{"id":"tc-1","name":"read_file","response":{"output":"hello world"}}}]}],"timestamp":"2026-09-01T12:01:00Z"},{"type":"user","content":"thanks","timestamp":"2026-09-01T12:02:00Z"},{"type":"gemini","content":"done","timestamp":"2026-09-01T12:03:00Z"}]}`)

	service := Service{HomeDir: home}
	original, err := service.Info("g1", "gemini")
	if err != nil {
		t.Fatal(err)
	}

	native := SerializeGemini(original)
	data, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}

	home2 := t.TempDir()
	geminiDir2 := filepath.Join(home2, ".gemini", "tmp", projectHash)
	writeFixture(t, filepath.Join(geminiDir2, ".project_root"), workspace)
	writeFixture(t, filepath.Join(geminiDir2, "chats", "session-g1.json"), string(data))

	service2 := Service{HomeDir: home2}
	roundTripped, err := service2.Info("g1", "gemini")
	if err != nil {
		t.Fatal(err)
	}

	assertMessagesEquivalent(t, original.Messages, roundTripped.Messages)

	// Verify tool call structure survived
	foundToolUse := false
	foundToolResult := false
	for _, m := range roundTripped.Messages {
		for _, b := range m.Content {
			if b.Type == "tool_use" && b.ID == "tc-1" && b.Name == "read_file" {
				foundToolUse = true
			}
		}
		if m.Role == "tool" && m.ToolUseID == "tc-1" {
			foundToolResult = true
			if textContent(m.Content) != "hello world" {
				t.Fatalf("tool result text = %q", textContent(m.Content))
			}
		}
	}
	if !foundToolUse {
		t.Fatal("missing tool_use block after gemini round-trip")
	}
	if !foundToolResult {
		t.Fatal("missing tool result after gemini round-trip")
	}
}

func testSerializePiRoundTrip(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "project")
	os.MkdirAll(workspace, 0o755)

	writeFixture(t, filepath.Join(home, ".pi", "agent", "sessions", "p", "session.jsonl"), strings.Join([]string{
		fmt.Sprintf(`{"type":"session","id":"pi1","cwd":%q,"timestamp":"2026-09-01T14:00:00Z"}`, workspace),
		`{"type":"message","message":{"id":"m1","role":"user","timestamp":"2026-09-01T14:00:00Z","content":[{"type":"text","text":"run ls"}]}}`,
		`{"type":"message","message":{"id":"m2","parentId":"m1","role":"assistant","timestamp":"2026-09-01T14:01:00Z","content":[{"type":"thinking","thinking":"reasoning","thinkingSignature":"sig-abc"},{"type":"text","text":"calling"},{"type":"toolCall","toolCallId":"pc-1","toolName":"shell","input":{"cmd":"ls"}}]}}`,
		`{"type":"message","message":{"role":"toolResult","timestamp":"2026-09-01T14:02:00Z","toolCallId":"pc-1","toolName":"shell","content":"file1.txt"}}`,
		`{"type":"message","message":{"id":"m4","parentId":"m2","role":"assistant","timestamp":"2026-09-01T14:03:00Z","content":[{"type":"text","text":"done"}]}}`,
	}, "\n"))

	detail := parseAndSerializeRoundTrip(t, home, "pi1", "pi", func(detail Detail) string {
		records := SerializePi(detail)
		return marshalJSONL(t, records)
	}, func(home2 string, content string) {
		writeFixture(t, filepath.Join(home2, ".pi", "agent", "sessions", "p", "session.jsonl"), content)
	})

	assertMessageRoles(t, detail.Messages, []string{"user", "assistant", "tool", "assistant"})
	assertBlockTypes(t, detail.Messages[1].Content, []string{"thinking", "text", "tool_use"})
	if detail.Messages[1].Content[0].Text != "reasoning" {
		t.Fatalf("thinking text = %q", detail.Messages[1].Content[0].Text)
	}
	if detail.Messages[1].Content[0].Signature != "sig-abc" {
		t.Fatalf("thinking signature = %q", detail.Messages[1].Content[0].Signature)
	}
	if detail.Messages[2].ToolUseID != "pc-1" || detail.Messages[2].ToolName != "shell" {
		t.Fatalf("tool message: useID=%q name=%q", detail.Messages[2].ToolUseID, detail.Messages[2].ToolName)
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
	home, id, provider string,
	serialize func(Detail) string,
	writeBack func(home2 string, content string),
) Detail {
	t.Helper()
	service := Service{HomeDir: home}
	original, err := service.Info(id, provider)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}

	content := serialize(original)

	home2 := t.TempDir()
	writeBack(home2, content)

	service2 := Service{HomeDir: home2}
	roundTripped, err := service2.Info(id, provider)
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
