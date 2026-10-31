package sessions

import (
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
