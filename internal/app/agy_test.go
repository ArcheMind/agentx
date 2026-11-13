package app

import (
	"bytes"
	"context"
	"github.com/ArcheMind/agentx/internal/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcheMind/agentx/internal/sessions"
)

func TestAgyConvertSQLite(t *testing.T) {
	var binary bytes.Buffer
	application := App{Output: OutputJSON, Stdin: strings.NewReader(`{"id":"source","provider":"claude","workspace":"/tmp","messages":[{"id":"u","role":"user","content":[{"type":"text","text":"remember this"}]}]}`), Stdout: &binary}
	if err := application.convertTo("agy"); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(binary.Bytes(), []byte("SQLite format 3\x00")) {
		t.Fatal("convert output must be native SQLite")
	}
	var unified bytes.Buffer
	application.Stdin, application.Stdout = bytes.NewReader(binary.Bytes()), &unified
	if err := application.convertFrom("agy"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(unified.String(), "remember this") {
		t.Fatal(unified.String())
	}
}

func TestAgyContinueStream(t *testing.T) {
	for _, test := range []struct {
		output    string
		failed    bool
		completed bool
	}{
		{`{"event":"init","conversation_id":"native"}` + "\n" + `{"event":"result","result":{"conversation_id":"native","status":"SUCCESS"}}`, false, true},
		{`{"event":"init","conversation_id":"wrong"}`, true, false},
		{`{"event":"result","result":{"conversation_id":"wrong","status":"SUCCESS"}}`, true, false},
		{`{"event":"result","result":{"conversation_id":"native","status":"ERROR","response":"rejected"}}`, false, false},
	} {
		run, err := parseContinueStream("agy", "future-model", "native", test.output)
		if (err != nil) != test.failed || run.completed != test.completed {
			t.Fatalf("run=%+v err=%v", run, err)
		}
	}
	if _, err := parseContinueStream("unknown", "", "", ""); err == nil {
		t.Fatal("unknown agent accepted")
	}
}

func TestAgyContinueNativePath(t *testing.T) {
	application := App{Sessions: sessions.Service{HomeDir: "/home/native"}}
	path, err := application.continueSessionFile("agy", "/workspace", "id", "")
	if err != nil || path != "/home/native/.gemini/antigravity-cli/conversations/id.db" {
		t.Fatalf("%s %v", path, err)
	}
}

func TestAgyMCPRequiresExplicitWorkspace(t *testing.T) {
	t.Setenv("SM_MCP_CWD", "")
	if err := (App{}).agyMCPServe(context.Background()); err == nil || !strings.Contains(err.Error(), "start agy through SessionMan") {
		t.Fatalf("%v", err)
	}
	plan := runtime.CommandPlan{Cwd: t.TempDir(), Env: map[string]string{"existing": "kept"}}
	prepareAgyNativePlan("agy", &plan)
	if plan.Env["SM_MCP_CWD"] != plan.Cwd || plan.Env["existing"] != "kept" {
		t.Fatalf("%+v", plan)
	}
}

func TestAgyCatalogPartialWarningKeepsUsableList(t *testing.T) {
	home := t.TempDir()
	valid := sessions.Session{Workspace: t.TempDir(), Messages: []sessions.Message{{ID: "u", Role: "user", Content: []sessions.ContentBlock{{Type: "text", Text: "usable transcript"}}}}}
	id, err := sessions.WriteAgySessionFile(valid, home)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", "broken.db")
	if err := os.WriteFile(bad, []byte("broken SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	application := New(false, strings.NewReader(""), &stdout, &stderr)
	application.Sessions = sessions.Service{HomeDir: home}
	if err := application.Run(context.Background(), []string{"--json", "session", "list", "--all"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), id) || !strings.Contains(stderr.String(), "partial session catalog") || !strings.Contains(stderr.String(), "broken.db") {
		t.Fatalf("stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}
