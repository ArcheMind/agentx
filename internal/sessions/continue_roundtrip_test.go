package sessions

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// deltaAppendHistory is the shape a continue caller accumulates: its own user
// messages plus returned delta messages as-is, with namespaced ids, dangling
// parentIds, and native model/stopReason fields.
func deltaAppendHistory() []Message {
	return []Message{
		{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "Remember the passphrase MANGO-42."}}},
		{
			ID:         "3f9a1c2e-aaaa-4bbb-8ccc-123456789abc:7ca6b465-9d24-444c-b290-547fd17454c6",
			ParentID:   "745042eb-2105-4ce7-915e-91b43874defd",
			Role:       "assistant",
			Timestamp:  "2026-10-31T18:57:03.527Z",
			Model:      "claude-fable-5",
			StopReason: "end_turn",
			Content:    []ContentBlock{{Type: "text", Text: "OK"}},
		},
		{ID: "u2", Role: "user", Content: []ContentBlock{{Type: "text", Text: "What was the passphrase?"}}},
	}
}

func TestRechainedClaudeRecordsStayOnOneAncestorChain(t *testing.T) {
	detail := Session{ID: "sid-1", Provider: "claude", Workspace: "/ws", Messages: RechainMessages(deltaAppendHistory())}
	records := SerializeClaude(detail)
	if len(records) != 3 {
		t.Fatalf("records = %d", len(records))
	}
	if records[0]["parentUuid"] != "" {
		t.Fatalf("first record parentUuid = %v; the root must have none", records[0]["parentUuid"])
	}
	for i := 1; i < len(records); i++ {
		parent := records[i]["parentUuid"]
		previous := records[i-1]["uuid"]
		if parent != previous {
			t.Fatalf("records[%d].parentUuid = %v, want previous uuid %v: Claude keeps only the newest record's ancestor chain, so a dangling parent orphans the history", i, parent, previous)
		}
	}
}

func TestClaudeDeltaAppendRoundTripKeepsHistory(t *testing.T) {
	detail := Session{ID: "sid-2", Provider: "claude", Workspace: "/ws", Messages: RechainMessages(deltaAppendHistory())}
	parsed, ok := ParseRecords("claude", SerializeClaude(detail))
	if !ok {
		t.Fatal("serialized claude session did not parse back")
	}
	assertRoundTripOrder(t, parsed.Messages)
}

func TestCodexDeltaAppendRoundTripKeepsHistory(t *testing.T) {
	detail := Session{ID: "3f9a1c2e-aaaa-4bbb-8ccc-123456789abc", Provider: "codex", Workspace: "/ws", Messages: RechainMessages(deltaAppendHistory())}
	parsed, ok := ParseRecords("codex", SerializeCodex(detail))
	if !ok {
		t.Fatal("serialized codex session did not parse back")
	}
	assertRoundTripOrder(t, parsed.Messages)
}

func assertRoundTripOrder(t *testing.T, messages []Message) {
	t.Helper()
	var texts []string
	var roles []string
	for _, message := range messages {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		roles = append(roles, message.Role)
		texts = append(texts, MessageText(message))
	}
	if len(texts) != 3 {
		t.Fatalf("round trip kept %d conversation messages (%v), want 3", len(texts), roles)
	}
	expected := []string{"Remember the passphrase MANGO-42.", "OK", "What was the passphrase?"}
	expectedRoles := []string{"user", "assistant", "user"}
	for i := range expected {
		if texts[i] != expected[i] || roles[i] != expectedRoles[i] {
			t.Fatalf("message %d = %s %q, want %s %q", i, roles[i], texts[i], expectedRoles[i], expected[i])
		}
	}
}

func TestRechainedPiRecordsStayOnOneAncestorChain(t *testing.T) {
	detail := Session{ID: "sid-3", Provider: "pi", Workspace: "/ws", Messages: RechainMessages(deltaAppendHistory())}
	records := SerializePi(detail)
	if len(records) != 4 {
		t.Fatalf("records = %d", len(records))
	}
	if records[1]["parentId"] != nil {
		t.Fatalf("first entry parentId = %v; the root must have none", records[1]["parentId"])
	}
	for i := 2; i < len(records); i++ {
		if records[i]["parentId"] != records[i-1]["id"] {
			t.Fatalf("records[%d].parentId = %v, want previous id %v", i, records[i]["parentId"], records[i-1]["id"])
		}
	}
}

func TestPiDeltaAppendRoundTripKeepsHistory(t *testing.T) {
	detail := Session{ID: "sid-4", Provider: "pi", Workspace: "/ws", Messages: RechainMessages(deltaAppendHistory())}
	parsed, ok := ParseRecords("pi", SerializePi(detail))
	if !ok {
		t.Fatal("serialized pi session did not parse back")
	}
	assertRoundTripOrder(t, parsed.Messages)
}

// toolHistory is the failing philia shape: the chain carries tool ids scoped
// by the pre-refactor harness ("native-<uuid>:<raw>", 76 chars; composite
// "call_x|fc_y") that Anthropic rejects on charset and OpenAI on length.
func toolHistory() []Message {
	scoped := "native-0d9c8b7a-6f5e-4d3c-2b1a-09f8e7d6c5b4:toolu_01Abc123Def456Ghi789Jkl0"
	composite := "call_AbCdEf123456|fc_9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a3928"
	return []Message{
		{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "Echo the marker MANGO-42."}}},
		{ID: "a1", Role: "assistant", Content: []ContentBlock{
			{Type: "tool_use", ID: scoped, Name: "Bash", Input: map[string]any{"command": "echo MANGO-42"}},
		}},
		{ID: "t1", Role: "tool", ToolUseID: scoped, ToolName: "Bash", Result: "MANGO-42"},
		{ID: "a2", Role: "assistant", Content: []ContentBlock{
			{Type: "tool_use", ID: composite, Name: "Read", Input: map[string]any{"file_path": "/x"}},
		}},
		{ID: "t2", Role: "tool", ToolUseID: composite, ToolName: "Read", Result: "ok"},
		{ID: "a3", Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "done"}}},
		{ID: "u2", Role: "user", Content: []ContentBlock{{Type: "text", Text: "What marker?"}}},
	}
}

func TestSanitizeToolIDsIsDeterministicPairedAndIdempotent(t *testing.T) {
	once := SanitizeToolIDs(toolHistory())
	again := SanitizeToolIDs(toolHistory())
	twice := SanitizeToolIDs(once)
	for i := range once {
		if once[i].ToolUseID != again[i].ToolUseID || once[i].ToolUseID != twice[i].ToolUseID {
			t.Fatalf("message %d: sanitization is not deterministic/idempotent", i)
		}
	}
	call1 := once[1].Content[0].ID
	call2 := once[3].Content[0].ID
	for _, id := range []string{call1, call2} {
		if !safeToolID.MatchString(id) {
			t.Fatalf("sanitized id %q is not API-safe", id)
		}
	}
	if call1 == call2 {
		t.Fatalf("distinct raw ids collapsed to one sanitized id")
	}
	if once[2].ToolUseID != call1 || once[4].ToolUseID != call2 {
		t.Fatalf("tool_use/tool_result pairing broken: %q/%q vs %q/%q", call1, once[2].ToolUseID, call2, once[4].ToolUseID)
	}
}

func TestSanitizeToolIDsKeepsNativeIdsVerbatim(t *testing.T) {
	native := []Message{
		{ID: "a1", Role: "assistant", Content: []ContentBlock{{Type: "tool_use", ID: "toolu_01GoodId", Name: "Bash", Input: map[string]any{}}}},
		{ID: "t1", Role: "tool", ToolUseID: "toolu_01GoodId", ToolName: "Bash", Result: "ok"},
		{ID: "a2", Role: "assistant", Content: []ContentBlock{{Type: "tool_use", ID: "call_abc123", Name: "exec", Input: map[string]any{}}}},
		{ID: "t2", Role: "tool", ToolUseID: "call_abc123", ToolName: "exec", Result: "ok"},
	}
	out := SanitizeToolIDs(native)
	if out[0].Content[0].ID != "toolu_01GoodId" || out[1].ToolUseID != "toolu_01GoodId" || out[2].Content[0].ID != "call_abc123" {
		t.Fatalf("native-safe ids must pass untouched: %+v", out)
	}
}

func TestClaudeToolHistoryImportIsAPISafe(t *testing.T) {
	messages := SanitizeToolIDs(RechainMessages(toolHistory()))
	records := SerializeClaude(Session{ID: "sid-5", Provider: "claude", Workspace: "/ws", Messages: messages})
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	ids := regexp.MustCompile(`"id":"([^"]+)"`).FindAllStringSubmatch(string(data), -1)
	uses := regexp.MustCompile(`"tool_use_id":"([^"]+)"`).FindAllStringSubmatch(string(data), -1)
	if len(ids) == 0 || len(uses) == 0 {
		t.Fatalf("expected tool ids in claude records: %s", data)
	}
	for _, match := range append(ids, uses...) {
		if !safeToolID.MatchString(match[1]) {
			t.Fatalf("claude record carries an API-unsafe tool id %q", match[1])
		}
	}
	parsed, ok := ParseRecords("claude", records)
	if !ok {
		t.Fatal("claude tool history did not parse back")
	}
	assertToolPairing(t, parsed.Messages)
}

func TestCodexToolHistoryImportIsAPISafe(t *testing.T) {
	messages := SanitizeToolIDs(RechainMessages(toolHistory()))
	records := SerializeCodex(Session{ID: "3f9a1c2e-aaaa-4bbb-8ccc-123456789abc", Provider: "codex", Workspace: "/ws", Messages: messages})
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`"call_id":"([^"]+)"`).FindAllStringSubmatch(string(data), -1) {
		if len(match[1]) > 64 || !safeToolID.MatchString(match[1]) {
			t.Fatalf("codex record carries an API-unsafe call_id %q (%d chars)", match[1], len(match[1]))
		}
	}
	parsed, ok := ParseRecords("codex", records)
	if !ok {
		t.Fatal("codex tool history did not parse back")
	}
	assertToolPairing(t, parsed.Messages)
}

func assertToolPairing(t *testing.T, messages []Message) {
	t.Helper()
	uses := map[string]bool{}
	for _, message := range messages {
		for _, block := range message.Content {
			if block.Type == "tool_use" {
				uses[block.ID] = true
			}
		}
	}
	results := 0
	for _, message := range messages {
		if message.Role != "tool" {
			continue
		}
		results++
		if !uses[message.ToolUseID] {
			t.Fatalf("tool result %q has no matching tool_use after round trip", message.ToolUseID)
		}
	}
	if len(uses) < 2 || results < 2 {
		t.Fatalf("round trip lost tool pairs: %d uses, %d results", len(uses), results)
	}
}

func TestSanitizeMessageIDsShortensForCodexAndKeepsChain(t *testing.T) {
	long := "01a1a806-1c2d-4e5f-8a9b-0c1d2e3f4a5b:msg_078c5d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b"
	messages := RechainMessages([]Message{
		{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}},
		{ID: long, Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "OK"}}},
		{ID: "u2", Role: "user", Content: []ContentBlock{{Type: "text", Text: "again"}}},
	})
	once := SanitizeMessageIDs(messages)
	twice := SanitizeMessageIDs(once)
	short := once[1].ID
	if len(short) != 64 || !strings.HasPrefix(short, "msg_") {
		t.Fatalf("long id not mapped to msg_+60 hex: %q", short)
	}
	if !strings.HasPrefix(once[0].ID, "msg_") || once[0].ID == short || SanitizeMessageIDs([]Message{{ID: "msg_abc"}})[0].ID != "msg_abc" || twice[1].ID != short || SanitizeMessageIDs(messages)[1].ID != short {
		t.Fatalf("shortening must be deterministic, idempotent and leave short ids verbatim: %+v", once)
	}
	if once[2].ParentID != short {
		t.Fatalf("parent chain broken: %q vs %q", once[2].ParentID, short)
	}
	data, err := json.Marshal(SerializeCodex(Session{ID: "sid-6", Provider: "codex", Workspace: "/ws", Messages: once}))
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`"id":"([^"]+)"`).FindAllStringSubmatch(string(data), -1) {
		if len(match[1]) > 64 {
			t.Fatalf("codex record carries an id over 64 chars: %q", match[1])
		}
	}
}
