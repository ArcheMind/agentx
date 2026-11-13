package sessions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func agyTestSession() Session {
	return Session{ID: "source-id", Provider: "claude", Workspace: "/tmp/agy-fixture", Title: "Agy fixture", StartedAt: "2026-11-13T12:00:00Z", Messages: []Message{
		{ID: "u", Role: "user", Timestamp: "2026-11-13T12:00:00Z", Content: []ContentBlock{{Type: "text", Text: "retain marker"}, {Type: "image", Source: map[string]any{"type": "base64", "media_type": "image/png", "data": base64.StdEncoding.EncodeToString([]byte("image bytes"))}}}},
		{ID: "a", Role: "assistant", Timestamp: "2026-11-13T12:00:01Z", Content: []ContentBlock{{Type: "thinking", Text: "readable thought", Signature: "foreign"}, {Type: "text", Text: "marker answer"}, {Type: "tool_use", ID: "call-1", Name: "research_tool", Input: map[string]any{"q": "value"}}}},
		{ID: "t", Role: "tool", ToolUseID: "call-1", ToolName: "research_tool", Result: "tool marker", Timestamp: "2026-11-13T12:00:02Z"},
	}}
}
func TestAgySourceFreeBootstrap(t *testing.T) {
	home := t.TempDir()
	source := agyTestSession()
	id, err := WriteAgySessionFile(source, home)
	if err != nil {
		t.Fatal(err)
	}
	if id == source.ID {
		t.Fatal("reused source ID")
	}
	path := filepath.Join(agyRoot(home), "conversations", id+".db")
	d, err := ParseAgySessionFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != id || d.Provider != "agy" || d.Workspace != source.Workspace {
		t.Fatalf("identity: %+v", d)
	}
	var thought, text, call, result bool
	for _, m := range d.Messages {
		for _, b := range m.Content {
			if b.Type == "thinking" {
				thought = b.Text == "readable thought" && b.Signature == ""
			}
			if b.Type == "text" && b.Text == "marker answer" {
				text = true
			}
			if b.Type == "tool_use" {
				call = b.ID == "call-1" && b.Name == "research_tool"
			}
		}
		if m.Role == "tool" {
			result = m.ToolUseID == "call-1" && m.Result == "tool marker"
		}
	}
	if !thought || !text || !call || !result {
		t.Fatalf("lost content: %+v", d.Messages)
	}
	if !reflect.DeepEqual(d.Messages[0].Content, source.Messages[0].Content) {
		t.Fatal("image/text changed")
	}
	db, err := agyOpen(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	for _, table := range []string{"gen_metadata", "executor_metadata", "parent_references", "battle_mode_infos"} {
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s: %d %v", table, n, err)
		}
	}
	var trajectoryID, cascadeID string
	if err := db.QueryRow(`SELECT trajectory_id,cascade_id FROM trajectory_meta`).Scan(&trajectoryID, &cascadeID); err != nil {
		t.Fatal(err)
	}
	if trajectoryID == id || cascadeID != id {
		t.Fatal("trajectory identity not independent")
	}
	sdb, err := agyOpen(filepath.Join(agyRoot(home), "conversation_summaries.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	var raw []byte
	if err := sdb.QueryRow(`SELECT raw_summary FROM conversation_summaries WHERE conversation_id=?`, id).Scan(&raw); err != nil || raw != nil {
		t.Fatalf("summary backup: %v %v", raw, err)
	}
	service := Service{HomeDir: home}
	list, err := service.List(ListOptions{Agent: "agy", All: true})
	if err != nil || len(list) != 1 || list[0].ID != id || list[0].Title != source.Title {
		t.Fatalf("list: %+v %v", list, err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Info(id, "agy")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("source database changed during read")
	}
	if err := WriteAgyDatabaseFile(source, path); err == nil {
		t.Fatal("overwrote existing database")
	}
}
func TestAgySignedThinking(t *testing.T) {
	for _, signature := range []string{"native-string", "agy:bytes:AAEC/w=="} {
		t.Run(signature, func(t *testing.T) {
			d := agyTestSession()
			d.Provider = "agy"
			d.Messages = d.Messages[1:2]
			d.Messages[0].Content = d.Messages[0].Content[:1]
			d.Messages[0].Content[0].Signature = signature
			path := filepath.Join(t.TempDir(), "native.db")
			if err := WriteAgyDatabaseFile(d, path); err != nil {
				t.Fatal(err)
			}
			actual, err := ParseAgySessionFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual.Messages[0].Content, d.Messages[0].Content) {
				t.Fatalf("signature changed: %+v", actual.Messages)
			}
		})
	}
	d := agyTestSession()
	d.Messages = []Message{{Role: "assistant", Content: []ContentBlock{{Type: "thinking", Signature: "opaque"}}}}
	steps, err := SerializeAgy(d)
	if err != nil || len(steps) != 0 {
		t.Fatalf("foreign opaque survived: %d %v", len(steps), err)
	}
}
func TestAgyWALAndPartialMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native.db")
	d := agyTestSession()
	if err := WriteAgyDatabaseFile(d, path); err != nil {
		t.Fatal(err)
	}
	db, err := agyOpen(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM trajectory_metadata_blob`); err != nil {
		t.Fatal(err)
	}
	step := agyJoin(agyInt(1, 14), agyBytes(19, agyText(2, "WAL marker")))
	if _, err = db.Exec(`INSERT INTO steps(idx,step_type,step_payload) VALUES (100,14,?)`, step); err != nil {
		t.Fatal(err)
	}
	actual, err := ParseAgySessionFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := textContent(actual.Messages[len(actual.Messages)-1].Content); got != "WAL marker" {
		t.Fatal(got)
	}
}
func TestAgyExternalResultHydration(t *testing.T) {
	dir := t.TempDir()
	resource := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(resource, []byte("full result marker"), 0600); err != nil {
		t.Fatal(err)
	}
	meta := agyBytes(4, agyJoin(agyText(1, "call"), agyText(2, "tool")))
	step := agyJoin(agyInt(1, 132), agyBytes(5, meta), agyBytes(140, agyBytes(2, agyJoin(agyText(1, "truncated"), agyText(7, "file://"+resource)))))
	p, err := agyDecode(step)
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseAgyStep(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.Result != "full result marker" {
		t.Fatal(m.Result)
	}
	d := Session{ID: "fresh", Provider: "agy", Messages: []Message{m}}
	path := filepath.Join(dir, "native.db")
	if err := WriteAgyDatabaseFile(d, path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(resource); err != nil {
		t.Fatal(err)
	}
	actual, err := ParseAgySessionFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Messages[0].Result != m.Result {
		t.Fatal("result depended on source file")
	}
}
func TestAgyMalformedDatabaseAndPayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAgySessionFile(path); err == nil {
		t.Fatal("accepted corrupt database")
	}
	for _, raw := range [][]byte{{0}, {0x0a, 0xff}, {0x08, 0x80}, {0x0b}} {
		if _, err := agyDecode(raw); err == nil {
			t.Fatalf("accepted %x", raw)
		}
	}
	path = filepath.Join(dir, "valid.db")
	if err := WriteAgyDatabaseFile(agyTestSession(), path); err != nil {
		t.Fatal(err)
	}
	db, err := agyOpen(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE steps SET step_payload=x'00' WHERE idx=0`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := ParseAgySessionFile(path); err == nil || !strings.Contains(err.Error(), "step 0") {
		t.Fatal(err)
	}
}
func TestAgyNativeDescriptorAnchor(t *testing.T) {
	raw, err := os.ReadFile("testdata/agy/descriptor-fields-v1.5.8.json")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]map[string]string
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, item := range [][3]string{{"CortexStepUserInput", "2", "user_response"}, {"CortexStepUserInput", "9", "media"}, {"CortexStepPlannerResponse", "14", "thinking_signature"}, {"CortexStepPlannerResponse", "7", "tool_calls"}, {"GenericStepResult", "7", "full_output_uri"}, {"CortexTrajectoryMetadata", "7", "workspace_uris"}} {
		if fields[item[0]][item[1]] != item[2] {
			t.Fatalf("descriptor changed: %v", item)
		}
	}
}

func TestAgyIndependentNativeFixture(t *testing.T) {
	d, err := ParseAgySessionFile("testdata/agy/native-v1.5.8.db")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "d6fc410a-5e5d-4f66-b6eb-c922e61a4dc0" || len(d.Messages) != 3 {
		t.Fatalf("fixture identity/count: %+v", d)
	}
	if got := d.Messages[1].Content; len(got) != 3 || got[0].Text != "Native readable thought" || got[0].Signature != "agy:bytes:AP9C" || got[1].Text != "Native answer body" || got[2].ID != "agy-tool-1" {
		t.Fatalf("fixture response: %+v", got)
	}
	if d.Messages[2].Result != "Native tool result body" || d.Messages[2].ToolUseID != "agy-tool-1" {
		t.Fatal("fixture tool linkage lost")
	}
}

func TestAgySubagentVisibilityAndFreshImport(t *testing.T) {
	home := t.TempDir()
	d := agyTestSession()
	d.Messages[0].Content = []ContentBlock{{Type: "text", Text: SubagentMarker + "\nDelegate a bounded task."}}
	id, err := WriteSessionFile(d, "agy", t.TempDir(), home)
	if err != nil {
		t.Fatal(err)
	}
	s := Service{HomeDir: home}
	hidden, err := s.List(ListOptions{Agent: "agy", All: true})
	if err != nil || len(hidden) != 0 {
		t.Fatalf("subagent visible: %+v %v", hidden, err)
	}
	visible, err := s.List(ListOptions{Agent: "agy", All: true, IncludeSubagents: true})
	if err != nil || len(visible) != 1 || !visible[0].IsSubagent {
		t.Fatalf("subagent missing: %+v %v", visible, err)
	}
	parsed, err := s.Info(id, "agy")
	if err != nil {
		t.Fatal(err)
	}
	again, err := WriteSessionFile(parsed, "agy", t.TempDir(), home)
	if err != nil || again == id {
		t.Fatalf("same-agent import reused identity: %s %v", again, err)
	}
	if got := ResumeArgs("agy", again); !reflect.DeepEqual(got, []string{"--conversation", again}) {
		t.Fatal(got)
	}
}
