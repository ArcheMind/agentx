package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgyHookSemantics(t *testing.T) {
	workspace := t.TempDir()
	payload, _ := json.Marshal(map[string]any{"conversationId": "native-id", "workspacePaths": []string{workspace}, "transcriptPath": "/native.db", "modelName": "future-model", "lastUserInput": "hello", "fullyIdle": true, "terminationReason": "complete"})
	event, err := Decode("agy", "Stop", payload)
	if err != nil {
		t.Fatal(err)
	}
	if event.Name != TurnAfter || event.SessionID != "native-id" || event.CWD != workspace || event.Model != "future-model" || event.TranscriptPath != "/native.db" {
		t.Fatalf("%+v", event)
	}
	for _, native := range []string{"SessionEnd", "UserPromptSubmit", "PreInvocation", "PostInvocation"} {
		if _, err := Decode("agy", native, payload); err == nil {
			t.Fatalf("incorrect portable mapping for %s", native)
		}
	}
	if _, err := agentEvent("agy", "SessionEnd"); err == nil {
		t.Fatal("SessionEnd projected")
	}
}

func TestAgyHookUsesActualWorkspace(t *testing.T) {
	workspace := t.TempDir()
	payload, _ := json.Marshal(map[string]any{"cwd": "/unusable/sessionman-test-path", "workspacePaths": []string{workspace}})
	event, err := Decode("agy", "SessionStart", payload)
	if err != nil || event.CWD != workspace {
		t.Fatalf("%+v %v", event, err)
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".agents"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".agents", "hooks.json"), []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo actual"}]}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := (Service{Home: home, Project: event.CWD}).List()
	if err != nil || len(list.Hooks) != 1 || !strings.Contains(list.Hooks[0].Command, "actual") {
		t.Fatalf("%+v %v", list, err)
	}
}
