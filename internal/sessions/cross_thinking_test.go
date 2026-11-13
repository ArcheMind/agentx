package sessions

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestThinkingReplayRespectsSourceAgent(t *testing.T) {
	for _, source := range []string{"claude", "codex", "pi", "unknown"} {
		for target, serialize := range map[string]func(Session) []map[string]any{
			"claude": SerializeClaude, "codex": SerializeCodex, "pi": SerializePi,
		} {
			t.Run(source+"->"+target, func(t *testing.T) {
				detail := Session{ID: "3f9a1c2e-aaaa-4bbb-8ccc-123456789abc", Provider: source, Workspace: "/w", StartedAt: "2026-11-12T12:00:00Z", Messages: []Message{
					{ID: "u1", Role: "user", Timestamp: "2026-11-12T12:00:00Z", Content: []ContentBlock{{Type: "text", Text: "remember"}}},
					{ID: "a1", ParentID: "u1", Role: "assistant", Timestamp: "2026-11-12T12:00:01Z", Content: []ContentBlock{
						{Type: "thinking", Text: "unique readable marker", Signature: "source-secret-signature"},
						{Type: "text", Text: "answer"},
					}},
					{ID: "a2", ParentID: "a1", Role: "assistant", Timestamp: "2026-11-12T12:00:02Z", Content: []ContentBlock{{Type: "thinking", Signature: "opaque-only-signature"}}},
					{ID: "u2", ParentID: "a2", Role: "user", Timestamp: "2026-11-12T12:00:03Z", Content: []ContentBlock{{Type: "text", Text: "recall"}}},
				}}
				encoded := marshalJSONL(t, serialize(detail))
				var records []map[string]any
				for _, line := range strings.Split(encoded, "\n") {
					var record map[string]any
					if err := json.Unmarshal([]byte(line), &record); err != nil {
						t.Fatal(err)
					}
					records = append(records, record)
				}
				parsed, ok := ParseRecords(target, records)
				if !ok {
					t.Fatal("native session failed to parse")
				}
				var got []ContentBlock
				for _, message := range parsed.Messages {
					if message.Role == "assistant" {
						got = append(got, message.Content...)
					}
				}
				want := []ContentBlock{{Type: "thinking", Text: "unique readable marker"}, {Type: "text", Text: "answer"}}
				if source == target {
					want[0].Signature = "source-secret-signature"
					want = append(want, ContentBlock{Type: "thinking", Signature: "opaque-only-signature"})
				} else if strings.Contains(encoded, "source-secret-signature") || strings.Contains(encoded, "opaque-only-signature") {
					t.Fatal("foreign signature reached the target native store")
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("blocks = %#v, want %#v", got, want)
				}
				if detail.Messages[1].Content[0].Signature != "source-secret-signature" {
					t.Fatal("source transcript was mutated")
				}
				if source != target && (target == "claude" || target == "pi") {
					last := parsed.Messages[len(parsed.Messages)-1]
					if last.ParentID != "a1" {
						t.Fatalf("dropped opaque-only thinking broke the parent chain: %+v", last)
					}
				}
			})
		}
	}
}
