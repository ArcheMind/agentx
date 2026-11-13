package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSessionFilePiLandsInNativeStore(t *testing.T) {
	home := t.TempDir()
	resumeDir := filepath.Join(home, ".local", "state", "sessionman", "resume")
	detail := Session{
		ID:        "imported",
		Provider:  "pi",
		Workspace: "/Users/agent/work",
		Messages: []Message{
			{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}},
			{ID: "a1", Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "ok"}}},
		},
	}

	path, err := WriteSessionFile(detail, "pi", resumeDir, home)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(home, ".pi", "agent", "sessions", "--Users-agent-work--")
	if filepath.Dir(path) != wantDir {
		t.Fatalf("path = %s, want directory %s", path, wantDir)
	}
	if !strings.HasSuffix(path, ".jsonl") {
		t.Fatalf("path = %s, want .jsonl", path)
	}
	records, err := readJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || records[0]["type"] != "session" || records[0]["cwd"] != "/Users/agent/work" {
		t.Fatalf("session record = %+v", records[0])
	}
	entries, err := os.ReadDir(resumeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("resume dir must stay empty, got %d entries", len(entries))
	}
}

func TestWriteSessionFilePiDefaultsWorkspaceToCwd(t *testing.T) {
	home := t.TempDir()
	detail := Session{ID: "imported", Provider: "pi", Messages: []Message{
		{ID: "u1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "hi"}}},
	}}
	path, err := WriteSessionFile(detail, "pi", t.TempDir(), home)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(filepath.Dir(path)), piSessionDirName(cwd); got != want {
		t.Fatalf("session dir = %s, want %s", got, want)
	}
	records, err := readJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || records[0]["cwd"] != cwd {
		t.Fatalf("session record = %+v", records[0])
	}
}

func TestPiSessionDirName(t *testing.T) {
	for workspace, want := range map[string]string{
		"/Users/agent/work": "--Users-agent-work--",
		`C:\Users\agent`:    "--C--Users-agent--",
		"/a/b:c":            "--a-b-c--",
	} {
		if got := piSessionDirName(workspace); got != want {
			t.Fatalf("piSessionDirName(%q) = %q, want %q", workspace, got, want)
		}
	}
}
